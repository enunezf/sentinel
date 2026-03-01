// Package bootstrap contiene la lógica de inicialización única del sistema Sentinel.
// Se ejecuta una sola vez cuando la base de datos está vacía (sin aplicaciones registradas)
// y crea los recursos mínimos necesarios para que el sistema funcione:
// aplicación "system", permisos de administración, rol "admin" y usuario administrador.
package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"

	"github.com/enunezf/sentinel/internal/config"
	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
)

// Initializer ejecuta el bootstrap único del sistema.
// Es idempotente: si ya existe al menos una aplicación en la base de datos,
// el bootstrap se omite sin error. Esto permite reiniciar el servicio de forma segura.
type Initializer struct {
	appRepo      *postgres.ApplicationRepository  // para verificar si el sistema ya fue inicializado
	userRepo     *postgres.UserRepository         // creación del usuario administrador inicial
	roleRepo     *postgres.RoleRepository         // creación del rol "admin" (is_system=true)
	permRepo     *postgres.PermissionRepository   // creación de los permisos de administración
	userRoleRepo *postgres.UserRoleRepository     // asignación del rol admin al usuario admin
	auditRepo    *postgres.AuditRepository        // registro del evento SYSTEM_BOOTSTRAP
	cfg          *config.Config                  // credenciales del admin (BOOTSTRAP_ADMIN_USER/PASSWORD)
	logger       *slog.Logger                    // logger estructurado con el campo "component=bootstrap"
}

// NewInitializer crea un Initializer con todas las dependencias de repositorio necesarias.
func NewInitializer(
	appRepo *postgres.ApplicationRepository,
	userRepo *postgres.UserRepository,
	roleRepo *postgres.RoleRepository,
	permRepo *postgres.PermissionRepository,
	userRoleRepo *postgres.UserRoleRepository,
	auditRepo *postgres.AuditRepository,
	cfg *config.Config,
	log *slog.Logger,
) *Initializer {
	return &Initializer{
		appRepo:      appRepo,
		userRepo:     userRepo,
		roleRepo:     roleRepo,
		permRepo:     permRepo,
		userRoleRepo: userRoleRepo,
		auditRepo:    auditRepo,
		cfg:          cfg,
		logger:       log.With("component", "bootstrap"),
	}
}

// Initialize ejecuta el bootstrap del sistema si este aún no ha sido inicializado.
// Es idempotente: retorna nil inmediatamente si ya existe alguna aplicación en la BD.
//
// El proceso de bootstrap sigue estos pasos en orden:
//  1. Verificar si ya hay aplicaciones (condición de guardián).
//  2. Validar las variables de entorno BOOTSTRAP_ADMIN_USER y BOOTSTRAP_ADMIN_PASSWORD.
//  3. Generar una secret_key aleatoria de 32 bytes (base64url) para la app "system".
//  4. Crear la aplicación "system".
//  5. Crear los 10 permisos de administración (admin.*).
//  6. Crear el rol "admin" con is_system=true y asignarle todos los permisos.
//  7. Crear el usuario administrador con la contraseña del entorno, hasheada con bcrypt.
//     NOTA: La contraseña del bootstrap NO se valida contra la política de seguridad.
//     Se establece must_change_pwd=true para forzar el cambio en el primer login.
//  8. Asignar el rol "admin" al usuario administrador en la app "system".
//  9. Insertar el evento SYSTEM_BOOTSTRAP en el log de auditoría (directamente, sin canal).
//
// Si cualquier paso falla, se retorna el error y el sistema queda en estado parcial
// (sin transacción global). En ese caso se debe limpiar la BD y reintentar.
func (i *Initializer) Initialize(ctx context.Context) error {
	// Paso 1: Verificar si el sistema ya fue inicializado.
	exists, err := i.appRepo.ExistsAny(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: check existing apps: %w", err)
	}
	if exists {
		i.logger.Info("bootstrap skipped, system already initialized")
		return nil
	}

	// Paso 2: Validar variables de entorno requeridas para el bootstrap.
	if i.cfg.Bootstrap.AdminUser == "" {
		return fmt.Errorf("bootstrap: BOOTSTRAP_ADMIN_USER is required but not set")
	}
	if i.cfg.Bootstrap.AdminPassword == "" {
		return fmt.Errorf("bootstrap: BOOTSTRAP_ADMIN_PASSWORD is required but not set")
	}

	i.logger.Info("starting system bootstrap")

	// Paso 3: Generar una secret_key criptográficamente aleatoria para la aplicación "system".
	// Esta clave se usa como X-App-Key para autenticar requests al servicio de autenticación.
	secretKey, err := generateSecretKey()
	if err != nil {
		return fmt.Errorf("bootstrap: generate secret key: %w", err)
	}

	// Paso 4: Crear la aplicación "system".
	app := &domain.Application{
		ID:        uuid.New(),
		Name:      "System",
		Slug:      "system",
		SecretKey: secretKey,
		IsActive:  true,
	}
	if err := i.appRepo.Create(ctx, app); err != nil {
		return fmt.Errorf("bootstrap: create system application: %w", err)
	}
	// Mostrar solo los primeros 8 caracteres de la secret_key en el log para facilitar
	// la depuración sin exponer la clave completa.
	i.logger.Info("system application created", "secret_key_hint", secretKey[:8]+"...")

	// Paso 5: Crear los permisos de administración del sistema.
	// Estos permisos cubren todas las operaciones del panel de administración.
	adminPerms := []struct{ code, desc, scope string }{
		{"admin.system.manage", "Full system administration", "global"},
		{"admin.users.read", "Read users", "resource"},
		{"admin.users.write", "Create/update users", "resource"},
		{"admin.roles.read", "Read roles", "resource"},
		{"admin.roles.write", "Create/update/delete roles", "resource"},
		{"admin.permissions.read", "Read permissions", "resource"},
		{"admin.permissions.write", "Create/delete permissions", "resource"},
		{"admin.cost_centers.read", "Read cost centers", "resource"},
		{"admin.cost_centers.write", "Create/update cost centers", "resource"},
		{"admin.audit.read", "Read audit logs", "resource"},
	}

	permIDs := make([]uuid.UUID, 0, len(adminPerms))
	for _, ap := range adminPerms {
		p := &domain.Permission{
			ID:            uuid.New(),
			ApplicationID: app.ID,
			Code:          ap.code,
			Description:   ap.desc,
			ScopeType:     domain.ScopeType(ap.scope),
		}
		if err := i.permRepo.Create(ctx, p); err != nil {
			return fmt.Errorf("bootstrap: create permission %s: %w", ap.code, err)
		}
		permIDs = append(permIDs, p.ID)
	}

	// Paso 6: Crear el rol "admin" como rol de sistema (is_system=true).
	// Los roles de sistema no pueden ser renombrados ni desactivados desde la API.
	adminRole := &domain.Role{
		ID:            uuid.New(),
		ApplicationID: app.ID,
		Name:          "admin",
		Description:   "System administrator role",
		IsSystem:      true,
		IsActive:      true,
	}
	if err := i.roleRepo.Create(ctx, adminRole); err != nil {
		return fmt.Errorf("bootstrap: create admin role: %w", err)
	}

	// Asignar todos los permisos de administración al rol "admin".
	for _, pid := range permIDs {
		if err := i.roleRepo.AddPermission(ctx, adminRole.ID, pid); err != nil {
			return fmt.Errorf("bootstrap: assign permission to admin role: %w", err)
		}
	}

	// Paso 7: Crear el usuario administrador.
	// La contraseña NO se valida contra la política de seguridad (exención del bootstrap
	// según la especificación). Se normaliza a NFC antes de hashear con bcrypt
	// para consistencia con el proceso normal de autenticación.
	// must_change_pwd=true obliga al admin a cambiar la contraseña en el primer login.
	normalizedPwd := norm.NFC.String(i.cfg.Bootstrap.AdminPassword)
	hash, err := bcrypt.GenerateFromPassword([]byte(normalizedPwd), i.cfg.Security.BcryptCost)
	if err != nil {
		return fmt.Errorf("bootstrap: hash admin password: %w", err)
	}

	adminUser := &domain.User{
		ID:            uuid.New(),
		Username:      i.cfg.Bootstrap.AdminUser,
		Email:         i.cfg.Bootstrap.AdminUser + "@system.local",
		PasswordHash:  string(hash),
		IsActive:      true,
		MustChangePwd: true,
	}
	if err := i.userRepo.Create(ctx, adminUser); err != nil {
		return fmt.Errorf("bootstrap: create admin user: %w", err)
	}
	i.logger.Info("admin user created", "username", adminUser.Username)

	// Paso 8: Asignar el rol "admin" al usuario administrador en la aplicación "system".
	// granted_by = adminUser.ID (auto-asignado durante el bootstrap).
	// valid_until = nil (sin expiración).
	now := time.Now()
	ur := &domain.UserRole{
		ID:            uuid.New(),
		UserID:        adminUser.ID,
		RoleID:        adminRole.ID,
		ApplicationID: app.ID,
		GrantedBy:     adminUser.ID, // auto-asignado; no hay otro actor disponible en bootstrap
		ValidFrom:     now,
		ValidUntil:    nil, // sin expiración para el administrador del sistema
	}
	if err := i.userRoleRepo.Assign(ctx, ur); err != nil {
		return fmt.Errorf("bootstrap: assign admin role to user: %w", err)
	}

	// Paso 9: Registrar el evento de bootstrap en el log de auditoría.
	// Se inserta directamente en la BD (sin pasar por el canal asíncrono de AuditService)
	// porque el AuditService puede no estar disponible en el momento del bootstrap.
	// Un fallo aquí es no-fatal: se registra como warning pero no aborta el bootstrap.
	resType := "application"
	auditLog := &domain.AuditLog{
		ID:            uuid.New(),
		EventType:     domain.EventSystemBootstrap,
		ApplicationID: &app.ID,
		ActorID:       &adminUser.ID,
		ResourceType:  &resType,
		ResourceID:    &app.ID,
		NewValue: map[string]interface{}{
			"application": "system",
			"admin_user":  adminUser.Username,
		},
		Success: true,
	}
	if err := i.auditRepo.Insert(ctx, auditLog); err != nil {
		i.logger.Warn("bootstrap audit log failed", "error", err)
	}

	i.logger.Info("system bootstrap completed")
	return nil
}

// generateSecretKey genera una secret_key criptográficamente aleatoria de 32 bytes
// codificada en base64url sin padding (43 caracteres).
// Se usa como la secret_key de la aplicación "system" durante el bootstrap.
func generateSecretKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
