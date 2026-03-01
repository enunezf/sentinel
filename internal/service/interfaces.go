package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	redisrepo "github.com/enunezf/sentinel/internal/repository/redis"
)

// UserRepositoryIface define los métodos de acceso a datos de usuarios que necesita
// AuthService. La implementación concreta es postgres.UserRepository; esta interfaz
// existe para permitir la inyección de mocks en pruebas unitarias.
type UserRepositoryIface interface {
	// FindByUsername devuelve el usuario cuyo nombre coincide con username, o nil si no existe.
	FindByUsername(ctx context.Context, username string) (*domain.User, error)

	// FindByID devuelve el usuario con el UUID indicado, o nil si no existe.
	FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error)

	// UpdateLastLogin actualiza la columna last_login_at del usuario al instante actual.
	// Se llama tras un login exitoso.
	UpdateLastLogin(ctx context.Context, userID uuid.UUID) error

	// UpdateFailedAttempts persiste el estado de bloqueo del usuario:
	// número de intentos fallidos, fecha hasta la que está bloqueado (nil = bloqueo permanente),
	// contador de lockouts del día y fecha del primer lockout.
	UpdateFailedAttempts(ctx context.Context, userID uuid.UUID, attempts int, lockedUntil *time.Time, lockoutCount int, lockoutDate *time.Time) error

	// UpdatePassword reemplaza el hash de contraseña del usuario con el nuevo hash bcrypt.
	UpdatePassword(ctx context.Context, userID uuid.UUID, hash string) error
}

// ApplicationRepositoryIface define los métodos de acceso a datos de aplicaciones
// que necesita AuthService. La implementación concreta es postgres.ApplicationRepository.
type ApplicationRepositoryIface interface {
	// FindBySecretKey devuelve la aplicación cuya secret_key coincide, o nil si no existe.
	// Se usa para validar el header X-App-Key en todos los endpoints protegidos.
	FindBySecretKey(ctx context.Context, secretKey string) (*domain.Application, error)

	// FindBySlug devuelve la aplicación cuyo slug coincide, o nil si no existe.
	FindBySlug(ctx context.Context, slug string) (*domain.Application, error)
}

// RefreshTokenPGRepositoryIface define las operaciones sobre refresh tokens en PostgreSQL.
// PostgreSQL es la fuente de verdad: almacena el bcrypt hash del token (nunca el valor raw).
type RefreshTokenPGRepositoryIface interface {
	// Create inserta un nuevo registro de refresh token. El campo TokenHash debe contener
	// el hash bcrypt del UUID v4 raw; nunca el valor en texto plano.
	Create(ctx context.Context, token *domain.RefreshToken) error

	// FindByHash busca el registro cuyo token_hash coincide con hash. Es el camino rápido:
	// se usa cuando Redis devuelve el hash almacenado junto con los metadatos del token.
	FindByHash(ctx context.Context, hash string) (*domain.RefreshToken, error)

	// FindByRawToken hace una búsqueda por fuerza bruta en PostgreSQL comparando bcrypt
	// contra todos los tokens activos. Es el camino lento (cache miss en Redis).
	FindByRawToken(ctx context.Context, rawToken string) (*domain.RefreshToken, error)

	// Revoke marca el token como revocado (is_revoked=true) por su ID.
	Revoke(ctx context.Context, id uuid.UUID) error

	// RevokeAllForUser revoca todos los refresh tokens activos de un usuario en una
	// aplicación concreta. Se llama desde Logout.
	RevokeAllForUser(ctx context.Context, userID, appID uuid.UUID) error
}

// RefreshTokenRedisRepositoryIface define las operaciones sobre refresh tokens en Redis.
// Redis actúa como caché de corta latencia: permite resolver el refresh token sin
// recurrir a PostgreSQL en el caso común.
type RefreshTokenRedisRepositoryIface interface {
	// Set almacena los metadatos del refresh token en Redis con la clave "refresh:<rawToken>".
	// TTL controla cuándo expira la entrada; debe coincidir con la duración del refresh token.
	Set(ctx context.Context, rawToken string, data redisrepo.RefreshTokenData, ttl time.Duration) error

	// Get recupera los metadatos del refresh token. Devuelve nil si la clave no existe o expiró.
	Get(ctx context.Context, rawToken string) (*redisrepo.RefreshTokenData, error)

	// Delete elimina la entrada del refresh token de Redis. Se llama al revocar o rotar el token.
	Delete(ctx context.Context, rawToken string) error
}

// PasswordHistoryRepositoryIface define las operaciones sobre el historial de contraseñas.
// La tabla password_history almacena los últimos N hashes bcrypt de cada usuario para
// impedir la reutilización de contraseñas recientes.
type PasswordHistoryRepositoryIface interface {
	// GetLastN devuelve los últimos n hashes bcrypt del usuario ordenados del más reciente
	// al más antiguo. Se usa para verificar que la nueva contraseña no haya sido usada antes.
	GetLastN(ctx context.Context, userID uuid.UUID, n int) ([]string, error)

	// Add inserta el hash actual del usuario en el historial antes de cambiarlo.
	Add(ctx context.Context, userID uuid.UUID, hash string) error
}

// UserRoleRepositoryIface define las operaciones de consulta de roles de usuario
// que necesita AuthService para incluir los roles activos en el access token.
type UserRoleRepositoryIface interface {
	// GetActiveRoleNamesForUserApp devuelve los nombres de los roles activos del usuario
	// en la aplicación indicada, respetando las fechas valid_from / valid_until.
	GetActiveRoleNamesForUserApp(ctx context.Context, userID, appID uuid.UUID) ([]string, error)
}

// AuditServiceIface define la operación de registro de eventos de auditoría.
// La implementación concreta es AuditService, que escribe eventos de forma asíncrona
// en un canal con buffer de tamaño 1000. Esta interfaz permite sustituirla por un mock
// en pruebas unitarias sin necesidad de una base de datos.
type AuditServiceIface interface {
	// LogEvent encola un evento de auditoría para su persistencia asíncrona.
	// Si el canal está lleno, el evento se descarta con un warning en el log;
	// nunca bloquea el flujo HTTP principal.
	LogEvent(entry *domain.AuditLog)
}

// AuthzUserContextRepositoryIface define las operaciones de caché de contexto de
// autorización. La implementación concreta es redisrepo.AuthzCache.
// La clave de caché es el JTI del access token, por lo que el contexto expira
// automáticamente cuando el token deja de ser válido.
type AuthzUserContextRepositoryIface interface {
	// GetPermissions recupera el contexto de usuario (permisos + centros de costo)
	// almacenado bajo el JTI del access token. Devuelve nil si no existe en caché.
	GetPermissions(ctx context.Context, jti string) (*redisrepo.UserContext, error)

	// SetPermissions almacena el contexto de usuario en caché con el TTL indicado.
	// El TTL debe coincidir con el tiempo de vida restante del access token.
	SetPermissions(ctx context.Context, jti string, uc *redisrepo.UserContext, ttl time.Duration) error
}
