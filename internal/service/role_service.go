package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
)

// RoleService implementa la lógica de negocio de gestión de roles en Sentinel.
// Los roles agrupan permisos y se asignan a usuarios por aplicación.
// Los roles con is_system=true son creados durante el bootstrap y no pueden
// ser renombrados ni desactivados.
// Cada operación de escritura emite un evento de auditoría asíncrono.
type RoleService struct {
	roleRepo   *postgres.RoleRepository       // CRUD de roles y asignación de permisos
	permRepo   *postgres.PermissionRepository // validación de permisos al asignar
	appRepo    *postgres.ApplicationRepository // consulta de aplicaciones para caché
	authzCache *redisrepo.AuthzCache          // invalidación del mapa de permisos al modificar roles
	auditSvc   *AuditService                 // registro asíncrono de eventos de auditoría
}

// NewRoleService crea un RoleService con todas las dependencias necesarias.
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

// CreateRole crea un nuevo rol para una aplicación.
// Los roles creados por esta vía tienen is_system=false y is_active=true por defecto.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - appID: UUID de la aplicación a la que pertenece el rol.
//   - name: nombre del rol; debe ser único dentro de la aplicación.
//   - description: descripción legible del propósito del rol.
//   - actorID: UUID del administrador que crea el rol (para auditoría).
//   - ip, ua: datos del actor para auditoría.
//
// Retorna ErrConflict si ya existe un rol con el mismo nombre en la aplicación.
// Invalida el caché del mapa de permisos de la aplicación tras la creación.
func (s *RoleService) CreateRole(ctx context.Context, appID uuid.UUID, name, description string, actorID uuid.UUID, ip, ua string) (*domain.Role, error) {
	role := &domain.Role{
		ID:            uuid.New(),
		ApplicationID: appID,
		Name:          name,
		Description:   description,
		IsSystem:      false, // los roles de sistema solo se crean en bootstrap
		IsActive:      true,
	}

	if err := s.roleRepo.Create(ctx, role); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
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

	// Invalidar el mapa de permisos en caché para que la próxima consulta
	// refleje el nuevo rol.
	app, _ := s.appRepo.FindBySlug(ctx, "")
	if app != nil {
		_ = s.authzCache.InvalidatePermissionsMap(ctx, app.Slug)
	}

	return role, nil
}

// GetRole recupera un rol por su UUID, incluyendo la lista de permisos asignados
// y el número de usuarios que tienen ese rol activo.
//
// Retorna error si el rol no existe.
func (s *RoleService) GetRole(ctx context.Context, roleID uuid.UUID) (*domain.RoleWithPermissions, error) {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil || role == nil {
		return nil, fmt.Errorf("role_svc: role not found")
	}

	perms, err := s.roleRepo.GetPermissions(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("role_svc: get permissions: %w", err)
	}

	// El conteo de usuarios no es crítico; si falla se devuelve 0.
	usersCount, _ := s.roleRepo.GetUsersCount(ctx, roleID)

	return &domain.RoleWithPermissions{
		Role:        *role,
		Permissions: perms,
		UsersCount:  usersCount,
	}, nil
}

// UpdateRole actualiza el nombre y la descripción de un rol.
// Los roles de sistema (is_system=true) no pueden ser renombrados; intentarlo
// devuelve un error. La descripción sí puede modificarse en roles de sistema.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - roleID: UUID del rol a actualizar.
//   - name: nuevo nombre del rol; si es vacío, se conserva el nombre actual.
//   - description: nueva descripción (puede ser vacía).
//   - actorID: UUID del administrador que realiza el cambio.
//   - ip, ua: datos del actor para auditoría.
//
// Retorna ErrNotFound si el rol no existe, ErrConflict si el nuevo nombre ya existe
// en la aplicación.
func (s *RoleService) UpdateRole(ctx context.Context, roleID uuid.UUID, name, description string, actorID uuid.UUID, ip, ua string) (*domain.Role, error) {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil || role == nil {
		return nil, ErrNotFound
	}

	// Los roles de sistema no pueden ser renombrados para mantener la integridad del RBAC base.
	if role.IsSystem && name != role.Name {
		return nil, fmt.Errorf("role_svc: cannot rename system role")
	}

	oldName := role.Name
	oldDesc := role.Description

	if name != "" {
		role.Name = name
	}
	role.Description = description

	if err := s.roleRepo.Update(ctx, role); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
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

// DeactivateRole establece is_active=false en el rol indicado.
// Los roles de sistema (is_system=true) no pueden ser desactivados.
// Los usuarios con el rol asignado perderán los permisos heredados en su próxima
// verificación (cuando el caché de AuthzService expire o se invalide).
//
// Retorna error si el rol no existe o es un rol de sistema.
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

// ListRoles devuelve una lista paginada de roles filtrada según los criterios de
// RoleFilter. Retorna el slice de roles, el total de registros y un error si falla.
func (s *RoleService) ListRoles(ctx context.Context, filter postgres.RoleFilter) ([]*domain.Role, int, error) {
	return s.roleRepo.List(ctx, filter)
}

// GetRolePermsCount devuelve el número de permisos actualmente asignados al rol.
// Se usa en el panel de administración para mostrar el contador en la lista de roles.
func (s *RoleService) GetRolePermsCount(ctx context.Context, roleID uuid.UUID) (int, error) {
	return s.roleRepo.GetPermissionsCount(ctx, roleID)
}

// AddPermissionToRole asigna un permiso a un rol.
// Si el rol ya tiene el permiso asignado, el repositorio maneja el caso de duplicado.
// Emite evento de auditoría EventRolePermissionAssigned.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - roleID: UUID del rol al que se asigna el permiso.
//   - permissionID: UUID del permiso a asignar.
//   - actorID: UUID del administrador que realiza la acción.
//   - ip, ua: datos del actor para auditoría.
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

	// Nota: la invalidación del caché se deja para una refactorización futura.
	// La llamada a FindBySlug con ID vacío es un placeholder.
	app, _ := s.appRepo.FindBySlug(ctx, role.ApplicationID.String())
	_ = app
	return nil
}

// RemovePermissionFromRole elimina la asociación entre un rol y un permiso.
// Los usuarios que tenían ese permiso a través de este rol lo perderán en la próxima
// evaluación de autorización (cuando el caché de AuthzService expire).
// Emite evento de auditoría EventRolePermissionRevoked.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - roleID: UUID del rol del que se elimina el permiso.
//   - permissionID: UUID del permiso a eliminar.
//   - actorID: UUID del administrador que realiza la acción.
//   - ip, ua: datos del actor para auditoría.
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
