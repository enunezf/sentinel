package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
)

// PermissionService implements permission management business logic.
type PermissionService struct {
	permRepo   *postgres.PermissionRepository
	appRepo    *postgres.ApplicationRepository
	authzCache *redisrepo.AuthzCache
	auditSvc   *AuditService
}

// NewPermissionService creates a PermissionService.
func NewPermissionService(
	permRepo *postgres.PermissionRepository,
	appRepo *postgres.ApplicationRepository,
	authzCache *redisrepo.AuthzCache,
	auditSvc *AuditService,
) *PermissionService {
	return &PermissionService{
		permRepo:   permRepo,
		appRepo:    appRepo,
		authzCache: authzCache,
		auditSvc:   auditSvc,
	}
}

// CreatePermission creates a new permission for an application.
func (s *PermissionService) CreatePermission(ctx context.Context, appID uuid.UUID, code, description, scopeType string, actorID uuid.UUID, ip, ua string) (*domain.Permission, error) {
	if !domain.IsValidScopeType(scopeType) {
		return nil, fmt.Errorf("%w: invalid scope_type: %s", ErrPermissionInvalid, scopeType)
	}

	p := &domain.Permission{
		ID:            uuid.New(),
		ApplicationID: appID,
		Code:          code,
		Description:   description,
		ScopeType:     domain.ScopeType(scopeType),
	}

	if err := s.permRepo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("perm_svc: create permission: %w", err)
	}

	appIDCopy := appID
	resType := "permission"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     domain.EventRoleCreated, // No specific permission created event; use admin log
		ApplicationID: &appIDCopy,
		ActorID:       &actorID,
		ResourceType:  &resType,
		ResourceID:    &p.ID,
		NewValue:      map[string]interface{}{"code": code, "scope_type": scopeType},
		IPAddress:     ip,
		UserAgent:     ua,
		Success:       true,
	})

	return p, nil
}

// GetPermission returns a permission by ID.
func (s *PermissionService) GetPermission(ctx context.Context, id uuid.UUID) (*domain.Permission, error) {
	p, err := s.permRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("perm_svc: find permission: %w", err)
	}
	return p, nil
}

// ListPermissions returns paginated permissions.
func (s *PermissionService) ListPermissions(ctx context.Context, filter postgres.PermissionFilter) ([]*domain.Permission, int, error) {
	return s.permRepo.List(ctx, filter)
}

// DeletePermission removes a permission (CASCADE in DB handles role_permissions, user_permissions).
func (s *PermissionService) DeletePermission(ctx context.Context, id uuid.UUID, actorID uuid.UUID, ip, ua string) error {
	if err := s.permRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("perm_svc: delete permission: %w", err)
	}

	resType := "permission"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:    domain.EventRoleDeleted, // Closest event type for permission deletion
		ActorID:      &actorID,
		ResourceType: &resType,
		ResourceID:   &id,
		IPAddress:    ip,
		UserAgent:    ua,
		Success:      true,
	})

	return nil
}

// ErrPermissionInvalid is returned when a permission field is invalid.
var ErrPermissionInvalid = fmt.Errorf("VALIDATION_ERROR")
