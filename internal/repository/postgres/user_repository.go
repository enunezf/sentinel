package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enunezf/sentinel/internal/domain"
)

// UserRepository handles persistence of User entities.
type UserRepository struct {
	db *pgxpool.Pool
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

// scanUser maps a pgx row into a domain.User.
func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	var lockoutDate *time.Time
	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.PasswordHash,
		&u.IsActive,
		&u.MustChangePwd,
		&u.LastLoginAt,
		&u.FailedAttempts,
		&u.LockedUntil,
		&u.LockoutCount,
		&lockoutDate,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	u.LockoutDate = lockoutDate
	return &u, nil
}

const userSelectFields = `
	id, username, email, password_hash, is_active, must_change_pwd,
	last_login_at, failed_attempts, locked_until, lockout_count, lockout_date,
	created_at, updated_at `

// FindByUsername returns a user by username or an error if not found.
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	q := `SELECT` + userSelectFields + `FROM users WHERE username = $1`
	row := r.db.QueryRow(ctx, q, username)
	u, err := scanUser(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("user_repo: find by username: %w", err)
	}
	return u, nil
}

// FindByID returns a user by UUID or nil if not found.
func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	q := `SELECT` + userSelectFields + `FROM users WHERE id = $1`
	row := r.db.QueryRow(ctx, q, id)
	u, err := scanUser(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("user_repo: find by id: %w", err)
	}
	return u, nil
}

// Create inserts a new user into the database.
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	const q = `
		INSERT INTO users (id, username, email, password_hash, is_active, must_change_pwd, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`
	_, err := r.db.Exec(ctx, q,
		user.ID, user.Username, user.Email, user.PasswordHash,
		user.IsActive, user.MustChangePwd,
	)
	if err != nil {
		return fmt.Errorf("user_repo: create: %w", err)
	}
	return nil
}

// UpdateFailedAttempts updates the lockout-related columns after a failed login.
func (r *UserRepository) UpdateFailedAttempts(ctx context.Context, userID uuid.UUID, attempts int, lockedUntil *time.Time, lockoutCount int, lockoutDate *time.Time) error {
	const q = `
		UPDATE users
		SET failed_attempts = $2,
		    locked_until    = $3,
		    lockout_count   = $4,
		    lockout_date    = $5,
		    updated_at      = NOW()
		WHERE id = $1`
	_, err := r.db.Exec(ctx, q, userID, attempts, lockedUntil, lockoutCount, lockoutDate)
	if err != nil {
		return fmt.Errorf("user_repo: update failed attempts: %w", err)
	}
	return nil
}

// UpdateLastLogin resets failed_attempts and locked_until on successful login.
func (r *UserRepository) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error {
	const q = `
		UPDATE users
		SET last_login_at   = NOW(),
		    failed_attempts = 0,
		    locked_until    = NULL,
		    updated_at      = NOW()
		WHERE id = $1`
	_, err := r.db.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("user_repo: update last login: %w", err)
	}
	return nil
}

// UpdatePassword sets a new password hash and clears must_change_pwd.
func (r *UserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, hash string) error {
	const q = `
		UPDATE users
		SET password_hash  = $2,
		    must_change_pwd = FALSE,
		    updated_at      = NOW()
		WHERE id = $1`
	_, err := r.db.Exec(ctx, q, userID, hash)
	if err != nil {
		return fmt.Errorf("user_repo: update password: %w", err)
	}
	return nil
}

// UpdatePasswordWithFlag sets the password hash and allows forcing must_change_pwd.
func (r *UserRepository) UpdatePasswordWithFlag(ctx context.Context, userID uuid.UUID, hash string, mustChangePwd bool) error {
	const q = `
		UPDATE users
		SET password_hash   = $2,
		    must_change_pwd  = $3,
		    updated_at       = NOW()
		WHERE id = $1`
	_, err := r.db.Exec(ctx, q, userID, hash, mustChangePwd)
	if err != nil {
		return fmt.Errorf("user_repo: update password with flag: %w", err)
	}
	return nil
}

// Unlock resets failed_attempts and locked_until for the user.
func (r *UserRepository) Unlock(ctx context.Context, userID uuid.UUID) error {
	const q = `
		UPDATE users
		SET failed_attempts = 0,
		    locked_until    = NULL,
		    lockout_count   = 0,
		    updated_at      = NOW()
		WHERE id = $1`
	_, err := r.db.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("user_repo: unlock: %w", err)
	}
	return nil
}

// UserFilter defines optional filters for listing users.
type UserFilter struct {
	Search   string
	IsActive *bool
	Page     int
	PageSize int
}

// List returns a paginated list of users and the total count.
func (r *UserRepository) List(ctx context.Context, filter UserFilter) ([]*domain.User, int, error) {
	args := []interface{}{}
	where := []string{}
	idx := 1

	if filter.Search != "" {
		where = append(where, fmt.Sprintf("(username ILIKE $%d OR email ILIKE $%d)", idx, idx+1))
		like := "%" + filter.Search + "%"
		args = append(args, like, like)
		idx += 2
	}
	if filter.IsActive != nil {
		where = append(where, fmt.Sprintf("is_active = $%d", idx))
		args = append(args, *filter.IsActive)
		idx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	countQ := `SELECT COUNT(*) FROM users ` + whereClause
	var total int
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("user_repo: list count: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize
	dataQ := `SELECT` + userSelectFields + `FROM users ` + whereClause +
		fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("user_repo: list query: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("user_repo: list scan: %w", err)
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// Update updates mutable user fields (username, email, is_active).
func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	const q = `
		UPDATE users
		SET username   = $2,
		    email      = $3,
		    is_active  = $4,
		    updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.Exec(ctx, q, user.ID, user.Username, user.Email, user.IsActive)
	if err != nil {
		return fmt.Errorf("user_repo: update: %w", err)
	}
	return nil
}
