// Package redis implementa los repositorios de caché y almacenamiento rápido de
// Sentinel usando el cliente go-redis v9. Los repositorios de este paquete usan
// Redis como capa de acceso rápido: los datos críticos también se persisten en
// PostgreSQL como fuente de verdad. Si Redis no está disponible, el sistema cae
// al fallback de PostgreSQL de forma transparente.
//
// Estrategia de claves Redis:
//   - Refresh tokens: "refresh:<uuid_raw>" → JSON con hash bcrypt y metadatos
//   - Contextos de usuario (permisos): "user_context:<jti>" → JSON con roles/permisos
//   - Mapa de permisos de app: "permissions_map:<slug>" → JSON firmado
//   - Versión del mapa de permisos: "permissions_map_version:<slug>" → hash de versión
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RefreshTokenData es la estructura que se almacena como valor en Redis para cada
// refresh token. La clave Redis es el UUID v4 raw del token (lo que se entrega al
// cliente). El valor contiene el hash bcrypt necesario para buscar el registro
// completo en PostgreSQL, además de metadatos del contexto de emisión.
//
// Diseño: guardar el hash en Redis permite buscar el registro en PostgreSQL por
// token_hash (columna indexada) sin necesidad de iterar todos los tokens activos.
type RefreshTokenData struct {
	UserID     string `json:"user_id"`     // UUID del usuario dueño del token
	AppID      string `json:"app_id"`      // UUID de la aplicación en cuyo contexto se emitió
	ExpiresAt  string `json:"expires_at"`  // timestamp ISO-8601 de expiración del token
	ClientType string `json:"client_type"` // tipo de cliente: "web" | "mobile" | "desktop"
	UserAgent  string `json:"user_agent"`  // User-Agent HTTP del cliente al momento del login
	IP         string `json:"ip"`          // dirección IP del cliente al momento del login
	TokenHash  string `json:"token_hash"`  // hash bcrypt del token; se usa para buscar en PostgreSQL
}

// RefreshTokenRepository gestiona el almacenamiento de refresh tokens en Redis.
// Redis actúa como caché de primer nivel: la validación del token busca primero
// aquí (O(1)) y solo consulta PostgreSQL si no encuentra el token (cache miss)
// o cuando necesita verificar el estado de revocación persistente.
type RefreshTokenRepository struct {
	client *redis.Client // cliente Redis; un único cliente por instancia de la aplicación
	logger *slog.Logger  // logger estructurado con component="redis_refresh_repo"
}

// NewRefreshTokenRepository construye un RefreshTokenRepository listo para usar.
// Recibe client, el cliente Redis inicializado, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewRefreshTokenRepository(client *redis.Client, log *slog.Logger) *RefreshTokenRepository {
	return &RefreshTokenRepository{
		client: client,
		logger: log.With("component", "redis_refresh_repo"),
	}
}

// refreshKey construye la clave Redis para un refresh token dado su UUID raw.
// El prefijo "refresh:" separa el espacio de nombres de refresh tokens de otras
// claves Redis (user_context:, permissions_map:, etc.) para facilitar la inspección
// y el borrado selectivo.
func refreshKey(hash string) string {
	return "refresh:" + hash
}

// Set almacena los datos de un refresh token en Redis con un TTL.
// Se llama inmediatamente después de generar un token nuevo, para que las
// validaciones subsiguientes puedan resolverse desde Redis sin tocar PostgreSQL.
// El TTL debe coincidir con la expiración del token: 7 días para clientes "web",
// 30 días para clientes "mobile" o "desktop" (definido en la capa de servicio).
// Parámetros:
//   - hash: UUID v4 raw del token (lo que se entregó al cliente); es la clave Redis.
//   - data: struct RefreshTokenData con el hash bcrypt y metadatos del token.
//   - ttl: tiempo de vida de la clave en Redis; debe ser mayor que cero.
func (r *RefreshTokenRepository) Set(ctx context.Context, hash string, data RefreshTokenData, ttl time.Duration) error {
	// Serializar el struct a JSON para almacenarlo como valor de cadena en Redis.
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("redis_refresh: marshal: %w", err)
	}
	// SET refresh:<hash> <json> EX <ttl_segundos>
	if err := r.client.Set(ctx, refreshKey(hash), string(b), ttl).Err(); err != nil {
		return fmt.Errorf("redis_refresh: set: %w", err)
	}
	return nil
}

// Get recupera los datos de un refresh token desde Redis por su UUID raw.
// Retorna nil (sin error) si el token no está en Redis: puede haberse expirado,
// no haberse almacenado nunca, o Redis puede haber reiniciado. En ese caso el
// caller debe caer al fallback de PostgreSQL.
// Parámetros:
//   - hash: UUID v4 raw del token (la clave Redis).
//
// Retorna los datos del token, nil si no existe, o un error de Redis.
func (r *RefreshTokenRepository) Get(ctx context.Context, hash string) (*RefreshTokenData, error) {
	val, err := r.client.Get(ctx, refreshKey(hash)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // cache miss — el caller debe consultar PostgreSQL
		}
		return nil, fmt.Errorf("redis_refresh: get: %w", err)
	}
	var data RefreshTokenData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, fmt.Errorf("redis_refresh: unmarshal: %w", err)
	}
	return &data, nil
}

// Delete elimina un refresh token de Redis por su UUID raw.
// Se llama durante el logout o cuando se revoca un token para que no pueda
// usarse de nuevo incluso si aún no expiró en Redis.
// Si la clave no existe, Redis no devuelve error (DEL es idempotente).
// Parámetros:
//   - hash: UUID v4 raw del token (la clave Redis a eliminar).
func (r *RefreshTokenRepository) Delete(ctx context.Context, hash string) error {
	// DEL refresh:<hash> — retorna el número de claves eliminadas (0 si no existía)
	if err := r.client.Del(ctx, refreshKey(hash)).Err(); err != nil {
		return fmt.Errorf("redis_refresh: delete: %w", err)
	}
	return nil
}
