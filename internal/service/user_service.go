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

// UserService implements user management business logic.
type UserService struct {
	userRepo     *postgres.UserRepository
	userRoleRepo *postgres.UserRoleRepository
	userPermRepo *postgres.UserPermissionRepository
	userCCRepo   *postgres.UserCostCenterRepository
	refreshRepo  *postgres.RefreshTokenRepository
	pwdHistRepo  *postgres.PasswordHistoryRepository
	appRepo      *postgres.ApplicationRepository
	auditSvc     *AuditService
	cfg          *config.Config
}

// NewUserService creates a UserService.
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

// CreateUserRequest holds input for user creation.
type CreateUserRequest struct {
	Username  string
	Email     string
	Password  string
	ActorID   uuid.UUID
	IP        string
	UserAgent string
}

// CreateUser creates a new user with bcrypt-hashed password.
func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*domain.User, error) {
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
		MustChangePwd: true,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
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

// GetUser retrieves a user by ID with roles, permissions, and cost centers.
func (s *UserService) GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("user_svc: find user: %w", err)
	}
	return user, nil
}

// UpdateUserRequest holds input for user update.
type UpdateUserRequest struct {
	Username  *string
	Email     *string
	IsActive  *bool
	ActorID   uuid.UUID
	IP        string
	UserAgent string
}

// UpdateUser applies partial updates to a user.
func (s *UserService) UpdateUser(ctx context.Context, userID uuid.UUID, req UpdateUserRequest) (*domain.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("user_svc: user not found")
	}

	oldValue := map[string]interface{}{
		"username":  user.Username,
		"email":     user.Email,
		"is_active": user.IsActive,
	}

	wasActive := user.IsActive

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

	// If deactivated, revoke all refresh tokens and emit USER_DEACTIVATED.
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

// ListUsers returns a paginated list of users.
func (s *UserService) ListUsers(ctx context.Context, filter postgres.UserFilter) ([]*domain.User, int, error) {
	return s.userRepo.List(ctx, filter)
}

// UnlockUser resets failed_attempts and locked_until for a user.
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

// ResetPassword generates a temporary password, updates it, and revokes tokens.
func (s *UserService) ResetPassword(ctx context.Context, userID uuid.UUID, actorID uuid.UUID, ip, ua string) (string, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil || user == nil {
		return "", fmt.Errorf("user_svc: user not found")
	}

	// Generate a temporary password that meets policy.
	tempPwd, err := generateTempPassword()
	if err != nil {
		return "", fmt.Errorf("user_svc: generate temp password: %w", err)
	}

	// Validate policy (should always pass for generated password).
	if err := ValidatePasswordPolicy(tempPwd); err != nil {
		return "", fmt.Errorf("user_svc: temp password policy: %w", err)
	}

	normalized := norm.NFC.String(tempPwd)
	hash, err := bcrypt.GenerateFromPassword([]byte(normalized), s.cfg.Security.BcryptCost)
	if err != nil {
		return "", fmt.Errorf("user_svc: hash temp password: %w", err)
	}

	// Save old hash to history.
	_ = s.pwdHistRepo.Add(ctx, userID, user.PasswordHash)

	if err := s.userRepo.UpdatePasswordWithFlag(ctx, userID, string(hash), true); err != nil {
		return "", fmt.Errorf("user_svc: update password: %w", err)
	}

	// Revoke all refresh tokens.
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

// AssignRoleRequest holds input for role assignment.
type AssignRoleRequest struct {
	RoleID     uuid.UUID
	ValidFrom  *time.Time
	ValidUntil *time.Time
	ActorID    uuid.UUID
	AppID      uuid.UUID
	IP         string
	UserAgent  string
}

// AssignRole assigns a role to a user.
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

	// Fetch the assignment with role name.
	assigned, err := s.userRoleRepo.FindByID(ctx, ur.ID)
	if err != nil {
		return ur, nil
	}
	return assigned, nil
}

// RevokeRole marks a user_role assignment as inactive.
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

// AssignPermissionRequest holds input for permission assignment.
type AssignPermissionRequest struct {
	PermissionID uuid.UUID
	ValidFrom    *time.Time
	ValidUntil   *time.Time
	ActorID      uuid.UUID
	AppID        uuid.UUID
	IP           string
	UserAgent    string
}

// AssignPermission assigns an individual permission to a user.
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

// RevokePermission marks a user_permission as inactive.
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

// AssignCostCentersRequest holds input for cost center assignment.
type AssignCostCentersRequest struct {
	CostCenterIDs []uuid.UUID
	ValidFrom     *time.Time
	ValidUntil    *time.Time
	ActorID       uuid.UUID
	AppID         uuid.UUID
	IP            string
	UserAgent     string
}

// AssignCostCenters assigns cost centers to a user.
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

// generateTempPassword generates a random password that meets the policy.
func generateTempPassword() (string, error) {
	for i := 0; i < 10; i++ {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		// Base64url gives mixed case + digits. Add a symbol.
		raw := base64.RawURLEncoding.EncodeToString(b)
		// Ensure uppercase.
		candidate := strings.ToUpper(raw[:1]) + raw[1:] + "!9"
		if ValidatePasswordPolicy(candidate) == nil {
			return candidate, nil
		}
	}
	// Fallback with guaranteed policy compliance.
	return "Temp@Pass1!", nil
}
