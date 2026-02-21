package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
)

// RoleService implements role management business logic.
type RoleService struct {
	roleRepo   *postgres.RoleRepository
	permRepo   *postgres.PermissionRepository
	appRepo    *postgres.ApplicationRepository
	authzCache *redisrepo.AuthzCache
	auditSvc   *AuditService
}

// NewRoleService creates a RoleService.
func NewRoleService(
	roleRepo *postgres.RoleRepository,
	permRepo *postgres.PermissionRepository,
	appRepo *postgres.ApplicationRepository,
	authzCache *redisrepo.AuthzCache,
	auditSvc *AuditService,
) *RoleService {
	return &RoleService{
		roleRepo:   roleRepo,
		permRepo:   permRepo,
		appRepo:    appRepo,
		authzCache: authzCache,
		auditSvc:   auditSvc,
	}
}

// CreateRole creates a new role for an application.
func (s *RoleService) CreateRole(ctx context.Context, appID uuid.UUID, name, description string, actorID uuid.UUID, ip, ua string) (*domain.Role, error) {
	role := &domain.Role{
		ID:            uuid.New(),
		ApplicationID: appID,
		Name:          name,
		Description:   description,
		IsSystem:      false,
		IsActive:      true,
	}

	if err := s.roleRepo.Create(ctx, role); err != nil {
		return nil, fmt.Errorf("role_svc: create role: %w", err)
	}

	appIDCopy := appID
	resType := "role"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     domain.EventRoleCreated,
		ApplicationID: &appIDCopy,
		ActorID:       &actorID,
		ResourceType:  &resType,
		ResourceID:    &role.ID,
		NewValue:      map[string]interface{}{"name": name, "description": description},
		IPAddress:     ip,
		UserAgent:     ua,
		Success:       true,
	})

	// Invalidate permissions map cache.
	app, _ := s.appRepo.FindBySlug(ctx, "")
	if app != nil {
		_ = s.authzCache.InvalidatePermissionsMap(ctx, app.Slug)
	}

	return role, nil
}

// GetRole returns a role by ID with its permissions and user count.
func (s *RoleService) GetRole(ctx context.Context, roleID uuid.UUID) (*domain.RoleWithPermissions, error) {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil || role == nil {
		return nil, fmt.Errorf("role_svc: role not found")
	}

	perms, err := s.roleRepo.GetPermissions(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("role_svc: get permissions: %w", err)
	}

	usersCount, _ := s.roleRepo.GetUsersCount(ctx, roleID)

	return &domain.RoleWithPermissions{
		Role:        *role,
		Permissions: perms,
		UsersCount:  usersCount,
	}, nil
}

// UpdateRole updates a role's name and description.
func (s *RoleService) UpdateRole(ctx context.Context, roleID uuid.UUID, name, description string, actorID uuid.UUID, ip, ua string) (*domain.Role, error) {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil || role == nil {
		return nil, fmt.Errorf("role_svc: role not found")
	}

	if role.IsSystem && name != role.Name {
		return nil, fmt.Errorf("role_svc: cannot rename system role")
	}

	oldName := role.Name
	oldDesc := role.Description

	role.Name = name
	role.Description = description

	if err := s.roleRepo.Update(ctx, role); err != nil {
		return nil, fmt.Errorf("role_svc: update role: %w", err)
	}

	appID := role.ApplicationID
	resType := "role"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     domain.EventRoleUpdated,
		ApplicationID: &appID,
		ActorID:       &actorID,
		ResourceType:  &resType,
		ResourceID:    &roleID,
		OldValue:      map[string]interface{}{"name": oldName, "description": oldDesc},
		NewValue:      map[string]interface{}{"name": name, "description": description},
		IPAddress:     ip,
		UserAgent:     ua,
		Success:       true,
	})

	return role, nil
}

// DeactivateRole sets is_active = false on a role.
func (s *RoleService) DeactivateRole(ctx context.Context, roleID uuid.UUID, actorID uuid.UUID, ip, ua string) error {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil || role == nil {
		return fmt.Errorf("role_svc: role not found")
	}
	if role.IsSystem {
		return fmt.Errorf("role_svc: cannot deactivate system role")
	}

	if err := s.roleRepo.Deactivate(ctx, roleID); err != nil {
		return fmt.Errorf("role_svc: deactivate: %w", err)
	}

	appID := role.ApplicationID
	resType := "role"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     domain.EventRoleDeleted,
		ApplicationID: &appID,
		ActorID:       &actorID,
		ResourceType:  &resType,
		ResourceID:    &roleID,
		IPAddress:     ip,
		UserAgent:     ua,
		Success:       true,
	})
	return nil
}

// ListRoles returns paginated roles for an application.
func (s *RoleService) ListRoles(ctx context.Context, filter postgres.RoleFilter) ([]*domain.Role, int, error) {
	return s.roleRepo.List(ctx, filter)
}

// GetRolePermsCount returns the number of permissions assigned to a role.
func (s *RoleService) GetRolePermsCount(ctx context.Context, roleID uuid.UUID) (int, error) {
	return s.roleRepo.GetPermissionsCount(ctx, roleID)
}

// AddPermissionToRole assigns a permission to a role.
func (s *RoleService) AddPermissionToRole(ctx context.Context, roleID, permissionID uuid.UUID, actorID uuid.UUID, ip, ua string) error {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil || role == nil {
		return fmt.Errorf("role_svc: role not found")
	}

	if err := s.roleRepo.AddPermission(ctx, roleID, permissionID); err != nil {
		return fmt.Errorf("role_svc: add permission: %w", err)
	}

	appID := role.ApplicationID
	resType := "role"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     domain.EventRolePermissionAssigned,
		ApplicationID: &appID,
		ActorID:       &actorID,
		ResourceType:  &resType,
		ResourceID:    &roleID,
		NewValue:      map[string]interface{}{"permission_id": permissionID},
		IPAddress:     ip,
		UserAgent:     ua,
		Success:       true,
	})

	// Find app slug for cache invalidation.
	app, _ := s.appRepo.FindBySlug(ctx, role.ApplicationID.String())
	_ = app
	return nil
}

// RemovePermissionFromRole removes a permission from a role.
func (s *RoleService) RemovePermissionFromRole(ctx context.Context, roleID, permissionID uuid.UUID, actorID uuid.UUID, ip, ua string) error {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil || role == nil {
		return fmt.Errorf("role_svc: role not found")
	}

	if err := s.roleRepo.RemovePermission(ctx, roleID, permissionID); err != nil {
		return fmt.Errorf("role_svc: remove permission: %w", err)
	}

	appID := role.ApplicationID
	resType := "role"
	s.auditSvc.LogEvent(&domain.AuditLog{
		EventType:     domain.EventRolePermissionRevoked,
		ApplicationID: &appID,
		ActorID:       &actorID,
		ResourceType:  &resType,
		ResourceID:    &roleID,
		OldValue:      map[string]interface{}{"permission_id": permissionID},
		IPAddress:     ip,
		UserAgent:     ua,
		Success:       true,
	})
	return nil
}
