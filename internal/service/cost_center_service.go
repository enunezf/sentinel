package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
)

// CostCenterService implements cost center management business logic.
type CostCenterService struct {
	ccRepo     *postgres.CostCenterRepository
	appRepo    *postgres.ApplicationRepository
	authzCache *redisrepo.AuthzCache
	auditSvc   *AuditService
}

// NewCostCenterService creates a CostCenterService.
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

// CreateCostCenter creates a new cost center for an application.
func (s *CostCenterService) CreateCostCenter(ctx context.Context, appID uuid.UUID, code, name string, actorID uuid.UUID, ip, ua string) (*domain.CostCenter, error) {
	cc := &domain.CostCenter{
		ID:            uuid.New(),
		ApplicationID: appID,
		Code:          code,
		Name:          name,
		IsActive:      true,
	}

	if err := s.ccRepo.Create(ctx, cc); err != nil {
		return nil, fmt.Errorf("cc_svc: create cost center: %w", err)
	}

	return cc, nil
}

// GetCostCenter retrieves a cost center by ID.
func (s *CostCenterService) GetCostCenter(ctx context.Context, id uuid.UUID) (*domain.CostCenter, error) {
	cc, err := s.ccRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("cc_svc: find cost center: %w", err)
	}
	return cc, nil
}

// UpdateCostCenter updates a cost center's name and active status.
func (s *CostCenterService) UpdateCostCenter(ctx context.Context, id uuid.UUID, name string, isActive bool, actorID uuid.UUID, ip, ua string) (*domain.CostCenter, error) {
	cc, err := s.ccRepo.FindByID(ctx, id)
	if err != nil || cc == nil {
		return nil, fmt.Errorf("cc_svc: cost center not found")
	}

	cc.Name = name
	cc.IsActive = isActive

	if err := s.ccRepo.Update(ctx, cc); err != nil {
		return nil, fmt.Errorf("cc_svc: update cost center: %w", err)
	}

	return cc, nil
}

// ListCostCenters returns paginated cost centers.
func (s *CostCenterService) ListCostCenters(ctx context.Context, filter postgres.CCFilter) ([]*domain.CostCenter, int, error) {
	return s.ccRepo.List(ctx, filter)
}
