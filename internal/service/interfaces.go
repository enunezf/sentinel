package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
)

// UserRepositoryIface defines the methods used by AuthService.
type UserRepositoryIface interface {
	FindByUsername(ctx context.Context, username string) (*domain.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	UpdateLastLogin(ctx context.Context, userID uuid.UUID) error
	UpdateFailedAttempts(ctx context.Context, userID uuid.UUID, attempts int, lockedUntil *time.Time, lockoutCount int, lockoutDate *time.Time) error
	UpdatePassword(ctx context.Context, userID uuid.UUID, hash string) error
}

// ApplicationRepositoryIface defines the methods used by AuthService.
type ApplicationRepositoryIface interface {
	FindBySecretKey(ctx context.Context, secretKey string) (*domain.Application, error)
	FindBySlug(ctx context.Context, slug string) (*domain.Application, error)
}

// RefreshTokenPGRepositoryIface defines PostgreSQL refresh token operations.
type RefreshTokenPGRepositoryIface interface {
	Create(ctx context.Context, token *domain.RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*domain.RefreshToken, error)
	FindByRawToken(ctx context.Context, rawToken string) (*domain.RefreshToken, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	RevokeAllForUser(ctx context.Context, userID, appID uuid.UUID) error
}

// RefreshTokenRedisRepositoryIface defines Redis refresh token operations.
type RefreshTokenRedisRepositoryIface interface {
	Set(ctx context.Context, rawToken string, data redisrepo.RefreshTokenData, ttl time.Duration) error
	Get(ctx context.Context, rawToken string) (*redisrepo.RefreshTokenData, error)
	Delete(ctx context.Context, rawToken string) error
}

// PasswordHistoryRepositoryIface defines password history operations.
type PasswordHistoryRepositoryIface interface {
	GetLastN(ctx context.Context, userID uuid.UUID, n int) ([]string, error)
	Add(ctx context.Context, userID uuid.UUID, hash string) error
}

// UserRoleRepositoryIface defines user role query operations.
type UserRoleRepositoryIface interface {
	GetActiveRoleNamesForUserApp(ctx context.Context, userID, appID uuid.UUID) ([]string, error)
}

// AuditServiceIface defines audit logging operations.
type AuditServiceIface interface {
	LogEvent(entry *domain.AuditLog)
}

// AuthzUserContextRepositoryIface defines authz cache operations used by AuthzService.
type AuthzUserContextRepositoryIface interface {
	GetPermissions(ctx context.Context, jti string) (*redisrepo.UserContext, error)
	SetPermissions(ctx context.Context, jti string, uc *redisrepo.UserContext, ttl time.Duration) error
}
