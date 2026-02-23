package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enunezf/sentinel/internal/domain"
)

// CostCenterRepository handles persistence of CostCenter entities.
type CostCenterRepository struct {
	db *pgxpool.Pool
}

// NewCostCenterRepository creates a new CostCenterRepository.
func NewCostCenterRepository(db *pgxpool.Pool) *CostCenterRepository {
	return &CostCenterRepository{db: db}
}

func scanCostCenter(row pgx.Row) (*domain.CostCenter, error) {
	var cc domain.CostCenter
	err := row.Scan(&cc.ID, &cc.ApplicationID, &cc.Code, &cc.Name, &cc.IsActive, &cc.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &cc, nil
}

// FindByID returns the cost center matching id, or nil.
func (r *CostCenterRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.CostCenter, error) {
	const q = `SELECT id, application_id, code, name, is_active, created_at FROM cost_centers WHERE id = $1`
	cc, err := scanCostCenter(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("cc_repo: find by id: %w", err)
	}
	return cc, nil
}

// Create inserts a new cost center.
func (r *CostCenterRepository) Create(ctx context.Context, cc *domain.CostCenter) error {
	const q = `
		INSERT INTO cost_centers (id, application_id, code, name, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())`
	_, err := r.db.Exec(ctx, q, cc.ID, cc.ApplicationID, cc.Code, cc.Name, cc.IsActive)
	if err != nil {
		return fmt.Errorf("cc_repo: create: %w", err)
	}
	return nil
}

// Update updates name and is_active of a cost center.
func (r *CostCenterRepository) Update(ctx context.Context, cc *domain.CostCenter) error {
	const q = `UPDATE cost_centers SET name = $2, is_active = $3 WHERE id = $1`
	_, err := r.db.Exec(ctx, q, cc.ID, cc.Name, cc.IsActive)
	if err != nil {
		return fmt.Errorf("cc_repo: update: %w", err)
	}
	return nil
}

// CCFilter defines filters for listing cost centers.
type CCFilter struct {
	ApplicationID *uuid.UUID
	Page          int
	PageSize      int
}

// List returns a paginated list of cost centers and total count.
func (r *CostCenterRepository) List(ctx context.Context, filter CCFilter) ([]*domain.CostCenter, int, error) {
	args := []interface{}{}
	where := ""
	if filter.ApplicationID != nil {
		where = "WHERE application_id = $1"
		args = append(args, *filter.ApplicationID)
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM cost_centers `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("cc_repo: list count: %w", err)
	}

	idx := len(args) + 1
	dataQ := `SELECT id, application_id, code, name, is_active, created_at FROM cost_centers ` +
		where + fmt.Sprintf(` ORDER BY code ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("cc_repo: list query: %w", err)
	}
	defer rows.Close()

	ccs := make([]*domain.CostCenter, 0)
	for rows.Next() {
		cc, err := scanCostCenter(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("cc_repo: scan: %w", err)
		}
		ccs = append(ccs, cc)
	}
	return ccs, total, rows.Err()
}

// ListByApp returns all cost centers for an application.
func (r *CostCenterRepository) ListByApp(ctx context.Context, appID uuid.UUID) ([]*domain.CostCenter, error) {
	const q = `SELECT id, application_id, code, name, is_active, created_at FROM cost_centers WHERE application_id = $1 ORDER BY code ASC`
	rows, err := r.db.Query(ctx, q, appID)
	if err != nil {
		return nil, fmt.Errorf("cc_repo: list by app: %w", err)
	}
	defer rows.Close()

	var ccs []*domain.CostCenter
	for rows.Next() {
		cc, err := scanCostCenter(rows)
		if err != nil {
			return nil, fmt.Errorf("cc_repo: scan: %w", err)
		}
		ccs = append(ccs, cc)
	}
	return ccs, rows.Err()
}
