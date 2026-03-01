// Package redis implementa los repositorios de caché de Sentinel usando go-redis v9.
// Ver refresh_token_repository.go para una descripción completa del paquete.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// UserContext es la estructura que se almacena en Redis como caché del contexto
// de permisos de un usuario. Se indexa por el JWT ID (jti) del token de acceso,
// de modo que cuando expira el token también se puede invalidar su caché.
// Contiene todos los datos necesarios para responder a una verificación de permiso
// sin tocar PostgreSQL.
type UserContext struct {
	UserID      string   `json:"user_id"`      // UUID del usuario autenticado
	Application string   `json:"application"`  // slug de la aplicación (e.g. "erp-interno")
	Roles       []string `json:"roles"`        // nombres de los roles activos del usuario en esta app
	Permissions []string `json:"permissions"`  // códigos de todos los permisos efectivos (roles + directos)
	CostCenters []string `json:"cost_centers"` // códigos de cost centers activos para el usuario en esta app
}

// AuthzCache gestiona las cachés de autorización en Redis.
// Almacena dos tipos de datos:
//  1. Contexto de usuario (user_context:<jti>): permisos, roles y cost centers
//     del usuario para el token JWT actual. TTL = tiempo de vida del JWT.
//  2. Mapa de permisos de aplicación (permissions_map:<slug>): JSON firmado con
//     todos los permisos definidos en la aplicación, usado por backends consumidores
//     para verificar permisos localmente sin llamar a Sentinel. TTL configurable.
//  3. Versión del mapa de permisos (permissions_map_version:<slug>): hash del
//     contenido del mapa, para que los backends detecten cambios sin descargar
//     el mapa completo.
type AuthzCache struct {
	client *redis.Client // cliente Redis; un único cliente por instancia de la aplicación
	logger *slog.Logger  // logger estructurado con component="authz_cache"
}

// NewAuthzCache construye un AuthzCache listo para usar.
// Recibe client, el cliente Redis inicializado, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewAuthzCache(client *redis.Client, log *slog.Logger) *AuthzCache {
	return &AuthzCache{
		client: client,
		logger: log.With("component", "authz_cache"),
	}
}

// userContextKey construye la clave Redis para el contexto de permisos de un usuario.
// El jti (JWT ID) es único por token de acceso, lo que permite invalidad la caché
// de un usuario específico sin afectar a otros.
func userContextKey(jti string) string {
	return "user_context:" + jti
}

// permissionsMapKey construye la clave Redis para el mapa de permisos de una aplicación.
// Se indexa por slug para facilitar la consulta desde backends consumidores que
// conocen el slug de la aplicación pero no su UUID.
func permissionsMapKey(appSlug string) string {
	return "permissions_map:" + appSlug
}

// permissionsMapVersionKey construye la clave Redis para la versión del mapa de permisos.
// Almacena un hash (firma RSA-SHA256 o hash SHA256) del contenido del mapa para
// que los backends puedan detectar si su copia local está desactualizada.
func permissionsMapVersionKey(appSlug string) string {
	return "permissions_map_version:" + appSlug
}

// SetPermissions almacena el contexto de permisos de un usuario en Redis,
// indexado por el JWT ID (jti) del token de acceso. El TTL debe coincidir con
// el tiempo de vida del JWT para que la caché expire al mismo tiempo que el token.
// Parámetros:
//   - jti: JWT ID único del token de acceso (campo "jti" del payload JWT).
//   - uc: contexto de permisos del usuario a cachear.
//   - ttl: tiempo de vida de la clave; normalmente igual al TTL del JWT (e.g. 15 minutos).
func (c *AuthzCache) SetPermissions(ctx context.Context, jti string, uc *UserContext, ttl time.Duration) error {
	b, err := json.Marshal(uc)
	if err != nil {
		return fmt.Errorf("authz_cache: marshal user context: %w", err)
	}
	// SET user_context:<jti> <json> EX <ttl_segundos>
	if err := c.client.Set(ctx, userContextKey(jti), string(b), ttl).Err(); err != nil {
		return fmt.Errorf("authz_cache: set permissions: %w", err)
	}
	return nil
}

// GetPermissions recupera el contexto de permisos de un usuario desde Redis
// por el JWT ID (jti). Retorna nil si la clave no existe (caché miss): puede
// que el token expiró, que Redis fue reiniciado, o que el usuario hizo logout.
// En ese caso el caller debe recalcular el contexto desde PostgreSQL.
// Parámetros:
//   - jti: JWT ID del token de acceso cuyo contexto se quiere recuperar.
//
// Retorna el contexto de permisos, nil si no existe en caché, o un error de Redis.
func (c *AuthzCache) GetPermissions(ctx context.Context, jti string) (*UserContext, error) {
	val, err := c.client.Get(ctx, userContextKey(jti)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // cache miss
		}
		return nil, fmt.Errorf("authz_cache: get permissions: %w", err)
	}
	var uc UserContext
	if err := json.Unmarshal([]byte(val), &uc); err != nil {
		return nil, fmt.Errorf("authz_cache: unmarshal user context: %w", err)
	}
	return &uc, nil
}

// DeletePermissions elimina el contexto de permisos de un usuario de Redis.
// Se llama durante el logout o cuando se detecta un cambio de permisos que
// requiere invalidar la caché (e.g. revocación de un rol o permiso).
// Si la clave no existe, Redis no devuelve error (DEL es idempotente).
// Parámetros:
//   - jti: JWT ID del token de acceso cuya caché se quiere eliminar.
func (c *AuthzCache) DeletePermissions(ctx context.Context, jti string) error {
	// DEL user_context:<jti>
	return c.client.Del(ctx, userContextKey(jti)).Err()
}

// SetPermissionsMap almacena el mapa completo de permisos de una aplicación en Redis.
// El mapa es un JSON canónico (claves ordenadas lexicográficamente, sin espacios)
// firmado con RSA-SHA256, que los backends consumidores pueden verificar localmente
// usando la clave pública de Sentinel (endpoint /.well-known/jwks.json).
// Parámetros:
//   - appSlug: slug de la aplicación (e.g. "erp-interno").
//   - mapData: JSON firmado del mapa de permisos (bytes crudos, no serializar de nuevo).
//   - ttl: tiempo de vida de la caché; configurable en config.yaml (e.g. 10 minutos).
func (c *AuthzCache) SetPermissionsMap(ctx context.Context, appSlug string, mapData []byte, ttl time.Duration) error {
	// SET permissions_map:<slug> <json_firmado> EX <ttl_segundos>
	if err := c.client.Set(ctx, permissionsMapKey(appSlug), string(mapData), ttl).Err(); err != nil {
		return fmt.Errorf("authz_cache: set permissions map: %w", err)
	}
	return nil
}

// GetPermissionsMap recupera el mapa de permisos en caché para una aplicación.
// Retorna nil si la clave no existe: el caller debe regenerar el mapa desde
// PostgreSQL y volver a cachearlo.
// Parámetros:
//   - appSlug: slug de la aplicación cuyo mapa se quiere recuperar.
//
// Retorna los bytes del JSON firmado, nil si no existe en caché, o un error de Redis.
func (c *AuthzCache) GetPermissionsMap(ctx context.Context, appSlug string) ([]byte, error) {
	val, err := c.client.Get(ctx, permissionsMapKey(appSlug)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // cache miss — el caller debe regenerar el mapa
		}
		return nil, fmt.Errorf("authz_cache: get permissions map: %w", err)
	}
	return []byte(val), nil
}

// SetPermissionsMapVersion almacena el hash de versión del mapa de permisos de una app.
// Los backends consumidores pueden consultar esta versión periódicamente para detectar
// si su copia local del mapa está desactualizada, sin necesidad de descargar el mapa
// completo en cada verificación.
// Parámetros:
//   - appSlug: slug de la aplicación.
//   - version: hash de la versión actual del mapa (e.g. SHA256 del JSON canónico).
//   - ttl: tiempo de vida de la clave de versión.
func (c *AuthzCache) SetPermissionsMapVersion(ctx context.Context, appSlug, version string, ttl time.Duration) error {
	// SET permissions_map_version:<slug> <hash_version> EX <ttl_segundos>
	if err := c.client.Set(ctx, permissionsMapVersionKey(appSlug), version, ttl).Err(); err != nil {
		return fmt.Errorf("authz_cache: set permissions map version: %w", err)
	}
	return nil
}

// GetPermissionsMapVersion recupera el hash de versión del mapa de permisos de una app.
// Retorna cadena vacía (sin error) si la clave no existe en Redis.
// Parámetros:
//   - appSlug: slug de la aplicación cuya versión se quiere consultar.
//
// Retorna el hash de versión, cadena vacía si no existe, o un error de Redis.
func (c *AuthzCache) GetPermissionsMapVersion(ctx context.Context, appSlug string) (string, error) {
	val, err := c.client.Get(ctx, permissionsMapVersionKey(appSlug)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil // no existe versión cacheada — el caller debe regenerar el mapa
		}
		return "", fmt.Errorf("authz_cache: get permissions map version: %w", err)
	}
	return val, nil
}

// InvalidatePermissionsMap elimina de Redis el mapa de permisos y su versión para una
// aplicación. Se llama cuando se modifica cualquier permiso de la aplicación (crear,
// eliminar, asignar a rol) para forzar la regeneración del mapa en la próxima consulta.
// Los errores de DEL se ignoran intencionalmente: la invalidación de caché es una
// operación de mejor esfuerzo; si falla, el mapa desactualizado expirará por TTL.
// Parámetros:
//   - appSlug: slug de la aplicación cuya caché de mapa se quiere invalidar.
func (c *AuthzCache) InvalidatePermissionsMap(ctx context.Context, appSlug string) error {
	// DEL permissions_map:<slug> — ignora error (best-effort)
	c.client.Del(ctx, permissionsMapKey(appSlug))
	// DEL permissions_map_version:<slug> — ignora error (best-effort)
	c.client.Del(ctx, permissionsMapVersionKey(appSlug))
	return nil
}
