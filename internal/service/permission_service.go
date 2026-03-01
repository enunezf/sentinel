package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
)

// PermissionService implementa la lógica de negocio de gestión de permisos en Sentinel.
// Los permisos son los átomos del sistema RBAC: se definen por aplicación, se asignan
// a roles y opcionalmente a usuarios individuales.
// Cada permiso tiene un scope_type que indica cómo se aplica la restricción:
//   - "global": el permiso aplica sin restricción de recurso (ej: "admin.system.manage").
//   - "resource": el permiso aplica sobre un tipo de recurso específico.
//   - "cost_center": el permiso aplica solo dentro de los centros de costo asignados al usuario.
type PermissionService struct {
	permRepo   *postgres.PermissionRepository // CRUD de permisos
	appRepo    *postgres.ApplicationRepository // consulta de aplicaciones (para invalidación de caché)
	authzCache *redisrepo.AuthzCache          // invalidación del mapa de permisos al modificar permisos
	auditSvc   *AuditService                 // registro asíncrono de eventos de auditoría
}

// NewPermissionService crea un PermissionService con todas las dependencias necesarias.
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

// CreatePermission crea un nuevo permiso para una aplicación.
// El código del permiso (code) debe ser único dentro de la aplicación.
// Se recomienda usar notación jerárquica (ej: "ventas.facturas.leer").
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - appID: UUID de la aplicación a la que pertenece el permiso.
//   - code: identificador único del permiso (ej: "admin.users.write").
//   - description: descripción legible del permiso.
//   - scopeType: tipo de alcance ("global" | "resource" | "cost_center").
//   - actorID: UUID del administrador que crea el permiso.
//   - ip, ua: datos del actor para auditoría.
//
// Retorna ErrPermissionInvalid si el scopeType no es válido.
// Retorna ErrConflict si ya existe un permiso con el mismo code en la aplicación.
func (s *PermissionService) CreatePermission(ctx context.Context, appID uuid.UUID, code, description, scopeType string, actorID uuid.UUID, ip, ua string) (*domain.Permission, error) {
	// Validar scope_type antes de intentar la inserción en base de datos.
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
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("perm_svc: create permission: %w", err)
	}

	appIDCopy := appID
	resType := "permission"
	s.auditSvc.LogEvent(&domain.AuditLog{
		// Nota: se usa EventRoleCreated como proxy; no existe un EventPermissionCreated dedicado.
		// En una iteración futura se puede agregar ese tipo de evento.
		EventType:     domain.EventRoleCreated,
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

// GetPermission recupera un permiso por su UUID.
// Retorna nil, nil si el permiso no existe (sin ErrNotFound).
// El handler es responsable de verificar si el resultado es nil.
func (s *PermissionService) GetPermission(ctx context.Context, id uuid.UUID) (*domain.Permission, error) {
	p, err := s.permRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("perm_svc: find permission: %w", err)
	}
	return p, nil
}

// ListPermissions devuelve una lista paginada de permisos filtrada según los criterios
// de PermissionFilter. Retorna el slice de permisos, el total de registros (para
// calcular total_pages) y un error si la consulta falla.
func (s *PermissionService) ListPermissions(ctx context.Context, filter postgres.PermissionFilter) ([]*domain.Permission, int, error) {
	return s.permRepo.List(ctx, filter)
}

// DeletePermission elimina un permiso de la base de datos.
// Las relaciones dependientes (role_permissions, user_permissions) se eliminan
// automáticamente por la cláusula ON DELETE CASCADE del esquema SQL.
// Emite evento de auditoría EventRoleDeleted (como proxy; no hay EventPermissionDeleted).
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - id: UUID del permiso a eliminar.
//   - actorID: UUID del administrador que realiza la eliminación.
//   - ip, ua: datos del actor para auditoría.
func (s *PermissionService) DeletePermission(ctx context.Context, id uuid.UUID, actorID uuid.UUID, ip, ua string) error {
	if err := s.permRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("perm_svc: delete permission: %w", err)
	}

	resType := "permission"
	s.auditSvc.LogEvent(&domain.AuditLog{
		// Se usa EventRoleDeleted como proxy al no existir EventPermissionDeleted.
		EventType:    domain.EventRoleDeleted,
		ActorID:      &actorID,
		ResourceType: &resType,
		ResourceID:   &id,
		IPAddress:    ip,
		UserAgent:    ua,
		Success:      true,
	})

	return nil
}

// ErrPermissionInvalid se devuelve cuando algún campo del permiso no es válido,
// por ejemplo cuando el scope_type no es uno de los valores permitidos.
var ErrPermissionInvalid = fmt.Errorf("VALIDATION_ERROR")
