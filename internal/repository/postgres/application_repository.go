package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enunezf/sentinel/internal/domain"
)

// ApplicationRepository handles persistence of Application entities.
type ApplicationRepository struct {
	db *pgxpool.Pool
}

// NewApplicationRepository creates a new ApplicationRepository.
func NewApplicationRepository(db *pgxpool.Pool) *ApplicationRepository {
	return &ApplicationRepository{db: db}
}

const appSelectFields = `id, name, slug, secret_key, is_active, created_at, updated_at`

func scanApp(row pgx.Row) (*domain.Application, error) {
	var a domain.Application
	err := row.Scan(&a.ID, &a.Name, &a.Slug, &a.SecretKey, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// FindBySecretKey returns the application matching the given secret_key, or nil.
func (r *ApplicationRepository) FindBySecretKey(ctx context.Context, secretKey string) (*domain.Application, error) {
	q := `SELECT ` + appSelectFields + ` FROM applications WHERE secret_key = $1`
	row := r.db.QueryRow(ctx, q, secretKey)
	a, err := scanApp(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("app_repo: find by secret key: %w", err)
	}
	return a, nil
}

// FindBySlug returns the application matching the slug, or nil.
func (r *ApplicationRepository) FindBySlug(ctx context.Context, slug string) (*domain.Application, error) {
	q := `SELECT ` + appSelectFields + ` FROM applications WHERE slug = $1`
	row := r.db.QueryRow(ctx, q, slug)
	a, err := scanApp(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("app_repo: find by slug: %w", err)
	}
	return a, nil
}

// Create inserts a new application.
func (r *ApplicationRepository) Create(ctx context.Context, app *domain.Application) error {
	const q = `
		INSERT INTO applications (id, name, slug, secret_key, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`
	_, err := r.db.Exec(ctx, q, app.ID, app.Name, app.Slug, app.SecretKey, app.IsActive)
	if err != nil {
		return fmt.Errorf("app_repo: create: %w", err)
	}
	return nil
}

// ExistsAny returns true if at least one application exists.
func (r *ApplicationRepository) ExistsAny(ctx context.Context) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM applications`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("app_repo: exists any: %w", err)
	}
	return count > 0, nil
}

// ApplicationFilter holds parameters for listing applications.
type ApplicationFilter struct {
	Search   string
	IsActive *bool
	Page     int
	PageSize int
}

// List returns a paginated list of applications and the total count.
func (r *ApplicationRepository) List(ctx context.Context, f ApplicationFilter) ([]*domain.Application, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}

	args := []interface{}{}
	where := "WHERE 1=1"
	i := 1

	if f.Search != "" {
		where += fmt.Sprintf(" AND (name ILIKE $%d OR slug ILIKE $%d)", i, i)
		args = append(args, "%"+f.Search+"%")
		i++
	}
	if f.IsActive != nil {
		where += fmt.Sprintf(" AND is_active = $%d", i)
		args = append(args, *f.IsActive)
		i++
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM applications `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("app_repo: list count: %w", err)
	}

	offset := (f.Page - 1) * f.PageSize
	args = append(args, f.PageSize, offset)
	q := `SELECT ` + appSelectFields + ` FROM applications ` + where +
		fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, i, i+1)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("app_repo: list query: %w", err)
	}
	defer rows.Close()

	var apps []*domain.Application
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("app_repo: list scan: %w", err)
		}
		apps = append(apps, a)
	}
	return apps, total, rows.Err()
}

// FindByID returns an application by its UUID, or nil if not found.
func (r *ApplicationRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Application, error) {
	q := `SELECT ` + appSelectFields + ` FROM applications WHERE id = $1`
	row := r.db.QueryRow(ctx, q, id)
	a, err := scanApp(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("app_repo: find by id: %w", err)
	}
	return a, nil
}

// Update changes the name and is_active fields of an application.
func (r *ApplicationRepository) Update(ctx context.Context, id uuid.UUID, name string, isActive bool) (*domain.Application, error) {
	const q = `
		UPDATE applications SET name = $2, is_active = $3, updated_at = NOW()
		WHERE id = $1
		RETURNING ` + appSelectFields
	row := r.db.QueryRow(ctx, q, id, name, isActive)
	a, err := scanApp(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("app_repo: update: %w", err)
	}
	return a, nil
}

// RotateSecretKey generates and persists a new secret_key for the application.
func (r *ApplicationRepository) RotateSecretKey(ctx context.Context, id uuid.UUID, newKey string) error {
	const q = `UPDATE applications SET secret_key = $2, updated_at = NOW() WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id, newKey)
	if err != nil {
		return fmt.Errorf("app_repo: rotate secret key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("app_repo: rotate secret key: application not found")
	}
	return nil
}
