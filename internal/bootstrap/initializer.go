package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"

	"github.com/enunezf/sentinel/internal/config"
	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
)

// Initializer runs the one-time system bootstrap.
type Initializer struct {
	appRepo      *postgres.ApplicationRepository
	userRepo     *postgres.UserRepository
	roleRepo     *postgres.RoleRepository
	permRepo     *postgres.PermissionRepository
	userRoleRepo *postgres.UserRoleRepository
	auditRepo    *postgres.AuditRepository
	cfg          *config.Config
}

// NewInitializer creates a new Initializer.
func NewInitializer(
	appRepo *postgres.ApplicationRepository,
	userRepo *postgres.UserRepository,
	roleRepo *postgres.RoleRepository,
	permRepo *postgres.PermissionRepository,
	userRoleRepo *postgres.UserRoleRepository,
	auditRepo *postgres.AuditRepository,
	cfg *config.Config,
) *Initializer {
	return &Initializer{
		appRepo:      appRepo,
		userRepo:     userRepo,
		roleRepo:     roleRepo,
		permRepo:     permRepo,
		userRoleRepo: userRoleRepo,
		auditRepo:    auditRepo,
		cfg:          cfg,
	}
}

// Initialize runs the bootstrap if the system has not been initialized yet.
// It is idempotent: if any application already exists, it returns nil immediately.
func (i *Initializer) Initialize(ctx context.Context) error {
	// Step 1: Check if already bootstrapped.
	exists, err := i.appRepo.ExistsAny(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: check existing apps: %w", err)
	}
	if exists {
		log.Println("INFO: bootstrap skipped – system already initialized")
		return nil
	}

	// Validate required env vars.
	if i.cfg.Bootstrap.AdminUser == "" {
		return fmt.Errorf("bootstrap: BOOTSTRAP_ADMIN_USER is required but not set")
	}
	if i.cfg.Bootstrap.AdminPassword == "" {
		return fmt.Errorf("bootstrap: BOOTSTRAP_ADMIN_PASSWORD is required but not set")
	}

	log.Println("INFO: starting system bootstrap")

	// Step 2: Generate a random secret_key for the "system" application.
	secretKey, err := generateSecretKey()
	if err != nil {
		return fmt.Errorf("bootstrap: generate secret key: %w", err)
	}

	// Step 3: Create application "system".
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
	log.Printf("INFO: system application created – SECRET KEY: %s (store this securely; shown only once)", secretKey)

	// Step 4: Create admin permissions.
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

	// Step 5: Create role "admin" (is_system=true).
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

	// Assign all admin permissions to the admin role.
	for _, pid := range permIDs {
		if err := i.roleRepo.AddPermission(ctx, adminRole.ID, pid); err != nil {
			return fmt.Errorf("bootstrap: assign permission to admin role: %w", err)
		}
	}

	// Step 6: Create admin user (skip password policy, must_change_pwd=true).
	// NOTE: Bootstrap password is NOT validated against policy per spec.
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
	log.Printf("INFO: admin user created – username: %s", adminUser.Username)

	// Step 7: Assign admin role to admin user for system application.
	now := time.Now()
	ur := &domain.UserRole{
		ID:            uuid.New(),
		UserID:        adminUser.ID,
		RoleID:        adminRole.ID,
		ApplicationID: app.ID,
		GrantedBy:     adminUser.ID, // self-assigned during bootstrap
		ValidFrom:     now,
		ValidUntil:    nil, // no expiration
	}
	if err := i.userRoleRepo.Assign(ctx, ur); err != nil {
		return fmt.Errorf("bootstrap: assign admin role to user: %w", err)
	}

	// Step 8: Register SYSTEM_BOOTSTRAP audit event.
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
		log.Printf("WARN: bootstrap audit log failed: %v", err)
	}

	log.Println("INFO: system bootstrap completed successfully")
	return nil
}

// generateSecretKey produces a cryptographically random base64url secret key.
func generateSecretKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
