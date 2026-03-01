package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
)

// CostCenterService implementa la lógica de negocio de gestión de centros de costo
// en Sentinel. Los centros de costo son entidades organizativas que restringen el
// acceso a datos cuando un permiso tiene scope_type = "cost_center". Por ejemplo,
// un usuario con permiso "ventas.facturas.leer" y centro de costo "CC-001" solo
// puede ver las facturas del centro de costo CC-001.
// Cada centro de costo pertenece a una aplicación específica.
type CostCenterService struct {
	ccRepo     *postgres.CostCenterRepository  // CRUD de centros de costo
	appRepo    *postgres.ApplicationRepository  // consulta de aplicaciones (para validaciones)
	authzCache *redisrepo.AuthzCache           // invalidación del mapa de permisos al modificar centros de costo
	auditSvc   *AuditService                  // registro asíncrono de eventos de auditoría
}

// NewCostCenterService crea un CostCenterService con todas las dependencias necesarias.
func NewCostCenterService(
	ccRepo *postgres.CostCenterRepository,
	appRepo *postgres.ApplicationRepository,
	authzCache *redisrepo.AuthzCache,
	auditSvc *AuditService,
) *CostCenterService {
	return &CostCenterService{
		ccRepo:     ccRepo,
		appRepo:    appRepo,
		authzCache: authzCache,
		auditSvc:   auditSvc,
	}
}

// CreateCostCenter crea un nuevo centro de costo para una aplicación.
// El código (code) debe ser único dentro de la aplicación.
// El centro de costo se crea con is_active=true por defecto.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - appID: UUID de la aplicación a la que pertenece el centro de costo.
//   - code: identificador único del centro de costo (ej: "CC-001", "VENTAS-MX").
//   - name: nombre legible del centro de costo.
//   - actorID: UUID del administrador que crea el centro de costo.
//   - ip, ua: datos del actor para auditoría.
//
// Retorna ErrConflict si ya existe un centro de costo con el mismo código en la aplicación.
func (s *CostCenterService) CreateCostCenter(ctx context.Context, appID uuid.UUID, code, name string, actorID uuid.UUID, ip, ua string) (*domain.CostCenter, error) {
	cc := &domain.CostCenter{
		ID:            uuid.New(),
		ApplicationID: appID,
		Code:          code,
		Name:          name,
		IsActive:      true,
	}

	if err := s.ccRepo.Create(ctx, cc); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("cc_svc: create cost center: %w", err)
	}

	return cc, nil
}

// GetCostCenter recupera un centro de costo por su UUID.
// Retorna nil, nil si no existe (sin ErrNotFound).
// El handler es responsable de verificar si el resultado es nil.
func (s *CostCenterService) GetCostCenter(ctx context.Context, id uuid.UUID) (*domain.CostCenter, error) {
	cc, err := s.ccRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("cc_svc: find cost center: %w", err)
	}
	return cc, nil
}

// UpdateCostCenter actualiza el nombre y el estado activo/inactivo de un centro de costo.
// El código (code) no es modificable para mantener la integridad de las asignaciones.
//
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - id: UUID del centro de costo a actualizar.
//   - name: nuevo nombre del centro de costo.
//   - isActive: nuevo estado; false desactiva el centro de costo.
//   - actorID: UUID del administrador que realiza el cambio.
//   - ip, ua: datos del actor para auditoría.
//
// Retorna ErrNotFound si el centro de costo no existe.
func (s *CostCenterService) UpdateCostCenter(ctx context.Context, id uuid.UUID, name string, isActive bool, actorID uuid.UUID, ip, ua string) (*domain.CostCenter, error) {
	cc, err := s.ccRepo.FindByID(ctx, id)
	if err != nil || cc == nil {
		return nil, ErrNotFound
	}

	cc.Name = name
	cc.IsActive = isActive

	if err := s.ccRepo.Update(ctx, cc); err != nil {
		return nil, fmt.Errorf("cc_svc: update cost center: %w", err)
	}

	return cc, nil
}

// ListCostCenters devuelve una lista paginada de centros de costo filtrada según
// los criterios de CCFilter. Retorna el slice de centros de costo, el total de
// registros (para calcular total_pages) y un error si la consulta falla.
func (s *CostCenterService) ListCostCenters(ctx context.Context, filter postgres.CCFilter) ([]*domain.CostCenter, int, error) {
	return s.ccRepo.List(ctx, filter)
}
