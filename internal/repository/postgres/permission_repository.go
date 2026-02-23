package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enunezf/sentinel/internal/domain"
)

// PermissionRepository handles persistence of Permission entities.
type PermissionRepository struct {
	db *pgxpool.Pool
}

// NewPermissionRepository creates a new PermissionRepository.
func NewPermissionRepository(db *pgxpool.Pool) *PermissionRepository {
	return &PermissionRepository{db: db}
}

func scanPermission(row pgx.Row) (*domain.Permission, error) {
	var p domain.Permission
	err := row.Scan(&p.ID, &p.ApplicationID, &p.Code, &p.Description, &p.ScopeType, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// FindByID returns the permission matching id, or nil.
func (r *PermissionRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Permission, error) {
	const q = `SELECT id, application_id, code, COALESCE(description,''), scope_type, created_at FROM permissions WHERE id = $1`
	p, err := scanPermission(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("perm_repo: find by id: %w", err)
	}
	return p, nil
}

// FindByCodeAndApp returns the permission matching code+appID, or nil.
func (r *PermissionRepository) FindByCodeAndApp(ctx context.Context, code string, appID uuid.UUID) (*domain.Permission, error) {
	const q = `SELECT id, application_id, code, COALESCE(description,''), scope_type, created_at FROM permissions WHERE code = $1 AND application_id = $2`
	p, err := scanPermission(r.db.QueryRow(ctx, q, code, appID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("perm_repo: find by code and app: %w", err)
	}
	return p, nil
}

// Create inserts a new permission.
func (r *PermissionRepository) Create(ctx context.Context, p *domain.Permission) error {
	const q = `
		INSERT INTO permissions (id, application_id, code, description, scope_type, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())`
	_, err := r.db.Exec(ctx, q, p.ID, p.ApplicationID, p.Code, p.Description, string(p.ScopeType))
	if err != nil {
		return fmt.Errorf("perm_repo: create: %w", err)
	}
	return nil
}

// Delete removes a permission by ID (CASCADE handles role_permissions, user_permissions).
func (r *PermissionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM permissions WHERE id = $1`
	_, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("perm_repo: delete: %w", err)
	}
	return nil
}

// PermissionFilter defines filters for listing permissions.
type PermissionFilter struct {
	ApplicationID *uuid.UUID
	Page          int
	PageSize      int
}

// List returns a paginated list of permissions.
func (r *PermissionRepository) List(ctx context.Context, filter PermissionFilter) ([]*domain.Permission, int, error) {
	args := []interface{}{}
	where := ""
	if filter.ApplicationID != nil {
		where = "WHERE application_id = $1"
		args = append(args, *filter.ApplicationID)
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM permissions `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("perm_repo: list count: %w", err)
	}

	idx := len(args) + 1
	dataQ := `SELECT id, application_id, code, COALESCE(description,''), scope_type, created_at FROM permissions ` +
		where + fmt.Sprintf(` ORDER BY code ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("perm_repo: list query: %w", err)
	}
	defer rows.Close()

	perms := make([]*domain.Permission, 0)
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("perm_repo: scan: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, total, rows.Err()
}

// ListByApp returns all permissions for an application (for permissions map).
func (r *PermissionRepository) ListByApp(ctx context.Context, appID uuid.UUID) ([]*domain.Permission, error) {
	const q = `SELECT id, application_id, code, COALESCE(description,''), scope_type, created_at FROM permissions WHERE application_id = $1 ORDER BY code ASC`
	rows, err := r.db.Query(ctx, q, appID)
	if err != nil {
		return nil, fmt.Errorf("perm_repo: list by app: %w", err)
	}
	defer rows.Close()

	var perms []*domain.Permission
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, fmt.Errorf("perm_repo: scan: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}
