package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"

	"github.com/enunezf/sentinel/internal/config"
	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
)

// UserService implementa la lógica de negocio de gestión de usuarios en Sentinel.
// Cubre el ciclo de vida completo: creación, actualización, listado, desbloqueo,
// restablecimiento de contraseña y gestión de roles, permisos y centros de costo.
// Cada operación de escritura emite un evento de auditoría asíncrono.
type UserService struct {
	userRepo     *postgres.UserRepository             // CRUD de usuarios
	userRoleRepo *postgres.UserRoleRepository         // asignación y revocación de roles
	userPermRepo *postgres.UserPermissionRepository   // asignación y revocación de permisos individuales
	userCCRepo   *postgres.UserCostCenterRepository   // asignación de centros de costo
	refreshRepo  *postgres.RefreshTokenRepository     // revocación de refresh tokens al desactivar usuario
	pwdHistRepo  *postgres.PasswordHistoryRepository  // historial de contraseñas para verificación de reutilización
	appRepo      *postgres.ApplicationRepository      // consulta de aplicaciones (para validaciones)
	auditSvc     *AuditService                       // registro asíncrono de eventos de auditoría
	cfg          *config.Config                      // configuración de seguridad (bcrypt cost, password history)
}

// NewUserService crea un UserService con todas las dependencias necesarias.
func NewUserService(
	userRepo *postgres.UserRepository,
	userRoleRepo *postgres.UserRoleRepository,
	userPermRepo *postgres.UserPermissionRepository,
	userCCRepo *postgres.UserCostCenterRepository,
	refreshRepo *postgres.RefreshTokenRepository,
	pwdHistRepo *postgres.PasswordHistoryRepository,
	appRepo *postgres.ApplicationRepository,
	auditSvc *AuditService,
	cfg *config.Config,
) *UserService {
	return &UserService{
		userRepo:     userRepo,
		userRoleRepo: userRoleRepo,
		userPermRepo: userPermRepo,
		userCCRepo:   userCCRepo,
		refreshRepo:  refreshRepo,
		pwdHistRepo:  pwdHistRepo,
		appRepo:      appRepo,
		auditSvc:     auditSvc,
		cfg:          cfg,
	}
}

// CreateUserRequest contiene los datos necesarios para crear un nuevo usuario.
type CreateUserRequest struct {
	Username  string    // nombre de usuario único en el sistema
	Email     string    // dirección de correo electrónico única
	Password  string    // contraseña en texto plano (se normaliza a NFC y se hashea con bcrypt)
	ActorID   uuid.UUID // UUID del administrador que realiza la creación (para auditoría)
	IP        string    // dirección IP del actor para auditoría
	UserAgent string    // User-Agent del actor para auditoría
}

// CreateUser crea un nuevo usuario con la contraseña hasheada con bcrypt.
// Proceso:
//  1. Normaliza la contraseña a Unicode NFC.
//  2. Valida la política de contraseña.
//  3. Hashea con bcrypt al costo configurado (cfg.Security.BcryptCost, típicamente 12).
//  4. Crea el usuario con is_active=true y must_change_pwd=true (fuerza cambio al primer login).
//  5. Emite evento de auditoría EventUserCreated.
//
// Retorna ErrConflict si username o email ya existen (unicidad en base de datos).
func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*domain.User, error) {
	// Normalizar a NFC para consistencia con la comparación bcrypt posterior.
	normalizedPwd := norm.NFC.String(req.Password)
	if err := ValidatePasswordPolicy(normalizedPwd); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(normalizedPwd), s.cfg.Security.BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("user_svc: hash password: %w", err)
	}

	user := &domain.User{
		ID:            uuid.New(),
		Username:      req.Username,
		Email:         req.Email,
		PasswordHash:  string(hash),
		IsActive:      true,
		MustChangePwd: true, // obliga al usuario a cambiar la contraseña en el primer login
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("user_svc: create user: %w", err)
	}

	actorID := req.ActorID
	userID := user.ID
	resType := "user"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:    domain.EventUserCreated,
		UserID:       &userID,
		ActorID:      &actorID,
		ResourceType: &resType,
		ResourceID:   &userID,
		NewValue:     map[string]interface{}{"username": user.Username, "email": user.Email},
		IPAddress:    req.IP,
		UserAgent:    req.UserAgent,
		Success:      true,
	})

	return user, nil
}

// GetUser recupera un usuario por su UUID.
// No devuelve ErrNotFound si el usuario no existe; devuelve nil, nil en ese caso.
// El handler es responsable de verificar si el resultado es nil.
func (s *UserService) GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("user_svc: find user: %w", err)
	}
	return user, nil
}

// UpdateUserRequest contiene los campos actualizables de un usuario.
// Los punteros permiten actualizaciones parciales: nil significa "no cambiar este campo".
type UpdateUserRequest struct {
	Username  *string   // nuevo nombre de usuario; nil = no cambiar
	Email     *string   // nuevo email; nil = no cambiar
	IsActive  *bool     // nuevo estado activo/inactivo; nil = no cambiar
	ActorID   uuid.UUID // UUID del administrador que realiza la actualización
	IP        string    // IP del actor para auditoría
	UserAgent string    // User-Agent del actor para auditoría
}

// UpdateUser aplica actualizaciones parciales a un usuario.
// Comportamiento especial: si is_active cambia de true a false (desactivación),
// se revocan automáticamente todos los refresh tokens del usuario en todas las
// aplicaciones y se emite un evento adicional EventUserDeactivated.
//
// Retorna ErrNotFound si el usuario no existe.
func (s *UserService) UpdateUser(ctx context.Context, userID uuid.UUID, req UpdateUserRequest) (*domain.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil {
		return nil, ErrNotFound
	}

	// Capturar los valores anteriores para el log de auditoría (old_value).
	oldValue := map[string]interface{}{
		"username":  user.Username,
		"email":     user.Email,
		"is_active": user.IsActive,
	}

	wasActive := user.IsActive

	// Aplicar solo los campos que se proporcionan (actualización parcial).
	if req.Username != nil {
		user.Username = *req.Username
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("user_svc: update user: %w", err)
	}

	newValue := map[string]interface{}{
		"username":  user.Username,
		"email":     user.Email,
		"is_active": user.IsActive,
	}

	actorID := req.ActorID
	resType := "user"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:    domain.EventUserUpdated,
		UserID:       &userID,
		ActorID:      &actorID,
		ResourceType: &resType,
		ResourceID:   &userID,
		OldValue:     oldValue,
		NewValue:     newValue,
		IPAddress:    req.IP,
		UserAgent:    req.UserAgent,
		Success:      true,
	})

	// Si el usuario fue desactivado, revocar todas sus sesiones activas.
	if wasActive && !user.IsActive {
		_ = s.refreshRepo.RevokeAllForUserAllApps(ctx, userID)
		s.auditSvc.LogEvent(&domain.AuditLog{
			EventType:    domain.EventUserDeactivated,
			UserID:       &userID,
			ActorID:      &actorID,
			ResourceType: &resType,
			ResourceID:   &userID,
			IPAddress:    req.IP,
			UserAgent:    req.UserAgent,
			Success:      true,
		})
	}

	return user, nil
}

// ListUsers devuelve una lista paginada de usuarios filtrada según los criterios de
// UserFilter. Retorna el slice de usuarios, el total de registros (para calcular
// total_pages) y un error si la consulta falla.
func (s *UserService) ListUsers(ctx context.Context, filter postgres.UserFilter) ([]*domain.User, int, error) {
	return s.userRepo.List(ctx, filter)
}

// UnlockUser restablece el contador de intentos fallidos y elimina el bloqueo
// temporal o permanente de un usuario. Solo puede ser ejecutado por un administrador.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - userID: UUID del usuario a desbloquear.
//   - actorID: UUID del administrador que realiza la acción.
//   - ip, ua: datos del administrador para auditoría.
func (s *UserService) UnlockUser(ctx context.Context, userID uuid.UUID, actorID uuid.UUID, ip, ua string) error {
	if err := s.userRepo.Unlock(ctx, userID); err != nil {
		return fmt.Errorf("user_svc: unlock user: %w", err)
	}

	resType := "user"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:    domain.EventUserUnlocked,
		UserID:       &userID,
		ActorID:      &actorID,
		ResourceType: &resType,
		ResourceID:   &userID,
		IPAddress:    ip,
		UserAgent:    ua,
		Success:      true,
	})
	return nil
}

// ResetPassword genera una contraseña temporal aleatoria, la hashea y actualiza
// al usuario con must_change_pwd=true. También revoca todos los refresh tokens del
// usuario en todas las aplicaciones para forzar un nuevo login.
//
// La contraseña temporal se genera como base64url de 16 bytes aleatorios con un
// sufijo "!9" para garantizar el cumplimiento de la política de contraseñas.
// El proceso intenta 10 veces antes de usar una contraseña de respaldo.
//
// Retorna la contraseña temporal en texto plano para que el administrador pueda
// comunicársela al usuario. NUNCA almacenar esta contraseña; se hashea en la BD.
func (s *UserService) ResetPassword(ctx context.Context, userID uuid.UUID, actorID uuid.UUID, ip, ua string) (string, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil {
		return "", fmt.Errorf("user_svc: user not found")
	}

	// Generar contraseña temporal aleatoria que cumpla la política.
	tempPwd, err := generateTempPassword()
	if err != nil {
		return "", fmt.Errorf("user_svc: generate temp password: %w", err)
	}

	// Validación defensiva: la contraseña generada siempre debe cumplir la política.
	if err := ValidatePasswordPolicy(tempPwd); err != nil {
		return "", fmt.Errorf("user_svc: temp password policy: %w", err)
	}

	normalized := norm.NFC.String(tempPwd)
	hash, err := bcrypt.GenerateFromPassword([]byte(normalized), s.cfg.Security.BcryptCost)
	if err != nil {
		return "", fmt.Errorf("user_svc: hash temp password: %w", err)
	}

	// Guardar el hash actual en el historial antes de reemplazarlo.
	_ = s.pwdHistRepo.Add(ctx, userID, user.PasswordHash)

	// Actualizar la contraseña y marcar must_change_pwd=true.
	if err := s.userRepo.UpdatePasswordWithFlag(ctx, userID, string(hash), true); err != nil {
		return "", fmt.Errorf("user_svc: update password: %w", err)
	}

	// Revocar todas las sesiones activas del usuario.
	_ = s.refreshRepo.RevokeAllForUserAllApps(ctx, userID)

	resType := "user"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:    domain.EventAuthPasswordReset,
		UserID:       &userID,
		ActorID:      &actorID,
		ResourceType: &resType,
		ResourceID:   &userID,
		IPAddress:    ip,
		UserAgent:    ua,
		Success:      true,
	})

	return tempPwd, nil
}

// AssignRoleRequest contiene los parámetros para asignar un rol a un usuario.
type AssignRoleRequest struct {
	RoleID     uuid.UUID  // UUID del rol a asignar
	ValidFrom  *time.Time // inicio de vigencia; nil = ahora
	ValidUntil *time.Time // fin de vigencia; nil = sin expiración
	ActorID    uuid.UUID  // UUID del administrador que asigna el rol
	AppID      uuid.UUID  // UUID de la aplicación a la que pertenece el rol
	IP         string     // IP del actor para auditoría
	UserAgent  string     // User-Agent del actor para auditoría
}

// AssignRole asigna un rol a un usuario en una aplicación específica.
// Si ValidFrom es nil, la asignación es efectiva desde el momento actual.
// Si ValidUntil es nil, la asignación no tiene fecha de expiración.
//
// Retorna el registro de asignación completo (con el nombre del rol si se puede
// recuperar) o el registro parcial si la consulta de confirmación falla.
func (s *UserService) AssignRole(ctx context.Context, userID uuid.UUID, req AssignRoleRequest) (*domain.UserRole, error) {
	validFrom := time.Now()
	if req.ValidFrom != nil {
		validFrom = *req.ValidFrom
	}

	ur := &domain.UserRole{
		ID:            uuid.New(),
		UserID:        userID,
		RoleID:        req.RoleID,
		ApplicationID: req.AppID,
		GrantedBy:     req.ActorID,
		ValidFrom:     validFrom,
		ValidUntil:    req.ValidUntil,
	}

	if err := s.userRoleRepo.Assign(ctx, ur); err != nil {
		return nil, fmt.Errorf("user_svc: assign role: %w", err)
	}

	resType := "role"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:    domain.EventUserRoleAssigned,
		UserID:       &userID,
		ActorID:      &req.ActorID,
		ResourceType: &resType,
		ResourceID:   &req.RoleID,
		NewValue:     map[string]interface{}{"role_id": req.RoleID, "user_id": userID},
		IPAddress:    req.IP,
		UserAgent:    req.UserAgent,
		Success:      true,
	})

	// Recuperar la asignación con el nombre del rol incluido.
	assigned, err := s.userRoleRepo.FindByID(ctx, ur.ID)
	if err != nil {
		// Si falla la consulta de confirmación, devolver el registro parcial.
		return ur, nil
	}
	return assigned, nil
}

// RevokeRole marca una asignación de rol como inactiva (is_active=false).
// No elimina el registro; mantiene el historial de asignaciones para auditoría.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - userID: UUID del usuario al que se le revoca el rol.
//   - assignmentID: UUID de la asignación (user_roles.id) a revocar.
//   - actorID: UUID del administrador que realiza la revocación.
//   - ip, ua: datos del actor para auditoría.
func (s *UserService) RevokeRole(ctx context.Context, userID, assignmentID uuid.UUID, actorID uuid.UUID, ip, ua string) error {
	if err := s.userRoleRepo.Revoke(ctx, assignmentID); err != nil {
		return fmt.Errorf("user_svc: revoke role: %w", err)
	}

	resType := "role"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:    domain.EventUserRoleRevoked,
		UserID:       &userID,
		ActorID:      &actorID,
		ResourceType: &resType,
		ResourceID:   &assignmentID,
		IPAddress:    ip,
		UserAgent:    ua,
		Success:      true,
	})
	return nil
}

// AssignPermissionRequest contiene los parámetros para asignar un permiso individual
// a un usuario (sin pasar por un rol).
type AssignPermissionRequest struct {
	PermissionID uuid.UUID  // UUID del permiso a asignar
	ValidFrom    *time.Time // inicio de vigencia; nil = ahora
	ValidUntil   *time.Time // fin de vigencia; nil = sin expiración
	ActorID      uuid.UUID  // UUID del administrador que asigna el permiso
	AppID        uuid.UUID  // UUID de la aplicación a la que pertenece el permiso
	IP           string     // IP del actor para auditoría
	UserAgent    string     // User-Agent del actor para auditoría
}

// AssignPermission asigna un permiso individual a un usuario en una aplicación.
// Los permisos individuales se suman a los permisos heredados por roles.
// Si ValidFrom es nil, la asignación es efectiva desde el momento actual.
func (s *UserService) AssignPermission(ctx context.Context, userID uuid.UUID, req AssignPermissionRequest) (*domain.UserPermission, error) {
	validFrom := time.Now()
	if req.ValidFrom != nil {
		validFrom = *req.ValidFrom
	}

	up := &domain.UserPermission{
		ID:            uuid.New(),
		UserID:        userID,
		PermissionID:  req.PermissionID,
		ApplicationID: req.AppID,
		GrantedBy:     req.ActorID,
		ValidFrom:     validFrom,
		ValidUntil:    req.ValidUntil,
	}

	if err := s.userPermRepo.Assign(ctx, up); err != nil {
		return nil, fmt.Errorf("user_svc: assign permission: %w", err)
	}

	resType := "permission"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:    domain.EventUserPermissionAssigned,
		UserID:       &userID,
		ActorID:      &req.ActorID,
		ResourceType: &resType,
		ResourceID:   &req.PermissionID,
		IPAddress:    req.IP,
		UserAgent:    req.UserAgent,
		Success:      true,
	})

	return up, nil
}

// RevokePermission marca una asignación individual de permiso como inactiva.
// No elimina el registro; mantiene el historial para auditoría.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - userID: UUID del usuario al que se le revoca el permiso.
//   - assignmentID: UUID de la asignación (user_permissions.id) a revocar.
//   - actorID: UUID del administrador que realiza la revocación.
//   - ip, ua: datos del actor para auditoría.
func (s *UserService) RevokePermission(ctx context.Context, userID, assignmentID uuid.UUID, actorID uuid.UUID, ip, ua string) error {
	if err := s.userPermRepo.Revoke(ctx, assignmentID); err != nil {
		return fmt.Errorf("user_svc: revoke permission: %w", err)
	}

	resType := "permission"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:    domain.EventUserPermissionRevoked,
		UserID:       &userID,
		ActorID:      &actorID,
		ResourceType: &resType,
		ResourceID:   &assignmentID,
		IPAddress:    ip,
		UserAgent:    ua,
		Success:      true,
	})
	return nil
}

// AssignCostCentersRequest contiene los parámetros para asignar centros de costo a
// un usuario. Se pueden asignar múltiples centros de costo en una sola llamada.
type AssignCostCentersRequest struct {
	CostCenterIDs []uuid.UUID // UUIDs de los centros de costo a asignar
	ValidFrom     *time.Time  // inicio de vigencia; nil = ahora
	ValidUntil    *time.Time  // fin de vigencia; nil = sin expiración
	ActorID       uuid.UUID   // UUID del administrador que asigna los centros de costo
	AppID         uuid.UUID   // UUID de la aplicación a la que pertenecen los centros de costo
	IP            string      // IP del actor para auditoría
	UserAgent     string      // User-Agent del actor para auditoría
}

// AssignCostCenters asigna uno o varios centros de costo a un usuario en una aplicación.
// Los centros de costo restringen qué datos puede acceder el usuario cuando un permiso
// tiene scope_type = "cost_center".
// Si alguna asignación falla, el proceso se detiene y retorna el error.
func (s *UserService) AssignCostCenters(ctx context.Context, userID uuid.UUID, req AssignCostCentersRequest) error {
	validFrom := time.Now()
	if req.ValidFrom != nil {
		validFrom = *req.ValidFrom
	}

	for _, ccID := range req.CostCenterIDs {
		ucc := &domain.UserCostCenter{
			UserID:        userID,
			CostCenterID:  ccID,
			ApplicationID: req.AppID,
			GrantedBy:     req.ActorID,
			ValidFrom:     validFrom,
			ValidUntil:    req.ValidUntil,
		}
		if err := s.userCCRepo.Assign(ctx, ucc); err != nil {
			return fmt.Errorf("user_svc: assign cost center %s: %w", ccID, err)
		}
	}

	resType := "cost_center"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:    domain.EventUserCostCenterAssigned,
		UserID:       &userID,
		ActorID:      &req.ActorID,
		ResourceType: &resType,
		IPAddress:    req.IP,
		UserAgent:    req.UserAgent,
		Success:      true,
	})
	return nil
}

// generateTempPassword genera una contraseña temporal aleatoria que cumple la política
// de seguridad de Sentinel: mínimo 10 caracteres, una mayúscula, un dígito, un símbolo.
//
// Estrategia:
//  1. Genera 16 bytes criptográficamente aleatorios.
//  2. Los codifica en base64url (produce letras, dígitos y "-", "_").
//  3. Fuerza el primer carácter a mayúscula y agrega "!9" al final para cumplir
//     los requisitos de símbolo y dígito.
//  4. Intenta hasta 10 veces; si todas fallan, usa la contraseña de respaldo.
func generateTempPassword() (string, error) {
	for i := 0; i < 10; i++ {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		// base64url produce una mezcla de letras y dígitos.
		raw := base64.RawURLEncoding.EncodeToString(b)
		// Garantizar: primera letra mayúscula + sufijo "!9" (símbolo + dígito).
		candidate := strings.ToUpper(raw[:1]) + raw[1:] + "!9"
		if ValidatePasswordPolicy(candidate) == nil {
			return candidate, nil
		}
	}
	// Contraseña de respaldo que garantiza el cumplimiento de la política en todos los casos.
	return "Temp@Pass1!", nil
}
