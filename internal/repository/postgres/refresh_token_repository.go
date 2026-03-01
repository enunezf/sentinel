// Package postgres implementa los repositorios de persistencia de Sentinel usando
// el driver pgx v5. Ver user_repository.go para una descripción completa del paquete.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/enunezf/sentinel/internal/domain"
)

// RefreshTokenRepository es el repositorio de la tabla "refresh_tokens" en PostgreSQL.
// Esta tabla actúa como almacenamiento persistente y de respaldo para los refresh tokens.
// El flujo principal de validación usa Redis (más rápido); PostgreSQL es la fuente de
// verdad y el fallback cuando Redis no está disponible.
//
// Diseño del almacenamiento:
//   - El UUID v4 raw (el token en texto claro que se entrega al cliente) es la clave
//     en Redis pero NUNCA se almacena en PostgreSQL.
//   - En PostgreSQL se guarda únicamente el hash bcrypt del token en la columna token_hash.
//   - La búsqueda por hash directo (FindByHash) es el método principal cuando se viene
//     desde Redis (que sí almacena la relación raw→hash).
//   - La búsqueda por token raw (FindByRawToken) es el fallback O(n) para cuando Redis
//     no está disponible; itera los tokens activos y compara con bcrypt.
//
// El campo device_info almacena metadatos del dispositivo del cliente como JSONB.
// Utiliza pgxpool.Pool para el pool de conexiones de pgx.
type RefreshTokenRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe
	logger *slog.Logger  // logger estructurado con component="refresh_token_repo"
}

// NewRefreshTokenRepository construye un RefreshTokenRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewRefreshTokenRepository(db *pgxpool.Pool, log *slog.Logger) *RefreshTokenRepository {
	return &RefreshTokenRepository{
		db:     db,
		logger: log.With("component", "refresh_token_repo"),
	}
}

// Create inserta un nuevo refresh token en la tabla "refresh_tokens".
// Se almacena el hash bcrypt del token (token.TokenHash), no el valor en texto claro.
// device_info se serializa a JSON para almacenarlo en la columna JSONB.
// is_revoked se inicializa en FALSE y created_at con NOW().
// Parámetros:
//   - token: struct domain.RefreshToken con todos los campos necesarios.
func (r *RefreshTokenRepository) Create(ctx context.Context, token *domain.RefreshToken) error {
	// Serializar device_info (metadata del dispositivo: tipo de cliente, OS, etc.) a JSON.
	deviceInfoJSON, err := json.Marshal(token.DeviceInfo)
	if err != nil {
		return fmt.Errorf("refresh_token_repo: marshal device_info: %w", err)
	}

	const q = `
		INSERT INTO refresh_tokens (id, user_id, app_id, token_hash, device_info, expires_at, is_revoked, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, FALSE, NOW())`
	// $1=id          — UUID de la fila del refresh token
	// $2=user_id     — usuario dueño del token
	// $3=app_id      — aplicación en cuyo contexto se emitió el token
	// $4=token_hash  — hash bcrypt del token raw (nunca el token en texto claro)
	// $5=device_info — JSON con metadatos del dispositivo
	// $6=expires_at  — timestamp de expiración (7d para web, 30d para mobile/desktop)
	_, err = r.db.Exec(ctx, q,
		token.ID, token.UserID, token.AppID, token.TokenHash,
		deviceInfoJSON, token.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("refresh_token_repo: create: %w", err)
	}
	return nil
}

// FindByHash busca un refresh token por su hash bcrypt en la tabla "refresh_tokens".
// Este es el método principal de lookup: cuando el servicio de autenticación recibe
// un token raw, primero lo busca en Redis (que guarda la relación raw→hash). Con el
// hash obtenido de Redis llama a este método para recuperar el registro completo de
// PostgreSQL (que contiene el estado de revocación, expiración, etc.).
// Parámetros:
//   - hash: hash bcrypt del token tal como se almacenó al crearlo.
//
// Retorna el refresh token encontrado, nil si no existe, o un error de base de datos.
func (r *RefreshTokenRepository) FindByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	const q = `
		SELECT id, user_id, app_id, token_hash, device_info, expires_at, used_at, is_revoked, created_at
		FROM refresh_tokens
		WHERE token_hash = $1`
	// $1 = token_hash (hash bcrypt almacenado)

	row := r.db.QueryRow(ctx, q, hash)

	var t domain.RefreshToken
	var deviceInfoRaw []byte // JSON raw de device_info; se deserializa después del scan
	err := row.Scan(
		&t.ID, &t.UserID, &t.AppID, &t.TokenHash,
		&deviceInfoRaw, &t.ExpiresAt, &t.UsedAt, &t.IsRevoked, &t.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("refresh_token_repo: find by hash: %w", err)
	}
	// Deserializar device_info de JSON a map/struct del dominio.
	if len(deviceInfoRaw) > 0 {
		_ = json.Unmarshal(deviceInfoRaw, &t.DeviceInfo)
	}
	return &t, nil
}

// Revoke marca un refresh token como revocado estableciendo is_revoked=TRUE.
// Se llama durante el logout o cuando se detecta un posible uso fraudulento
// (token replay attack).
// Parámetros:
//   - id: UUID de la fila del refresh token a revocar.
func (r *RefreshTokenRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE refresh_tokens SET is_revoked = TRUE WHERE id = $1`
	// $1 = id (UUID de la fila)
	_, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("refresh_token_repo: revoke: %w", err)
	}
	return nil
}

// RevokeAllForUser revoca todos los refresh tokens activos de un usuario en una
// aplicación específica. Se llama al cambiar la contraseña o al detectar actividad
// sospechosa, para forzar al usuario a re-autenticarse en todos sus dispositivos
// de esa aplicación.
// Parámetros:
//   - userID: UUID del usuario cuyos tokens se revocan.
//   - appID: UUID de la aplicación (solo se revocan los tokens de esa app).
func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID, appID uuid.UUID) error {
	const q = `UPDATE refresh_tokens SET is_revoked = TRUE WHERE user_id = $1 AND app_id = $2 AND is_revoked = FALSE`
	// $1=user_id, $2=app_id; el filtro is_revoked=FALSE evita actualizar filas ya revocadas
	_, err := r.db.Exec(ctx, q, userID, appID)
	if err != nil {
		return fmt.Errorf("refresh_token_repo: revoke all for user: %w", err)
	}
	return nil
}

// FindByRawToken busca un refresh token comparando el token raw contra los hashes
// bcrypt almacenados. Este método es el FALLBACK cuando Redis no está disponible.
// Es O(n) en el número de tokens activos, por lo que se limita a los 1000 tokens
// más recientes para acotar el impacto en latencia.
//
// Proceso: itera los tokens no revocados y no expirados, y para cada uno llama a
// bcrypt.CompareHashAndPassword. La primera coincidencia se devuelve inmediatamente.
// bcrypt es un algoritmo costoso (intencional, para proteger contra ataques de
// fuerza bruta), por lo que este fallback puede ser lento si hay muchos tokens activos.
//
// Parámetros:
//   - rawToken: el UUID v4 en texto claro que el cliente envió en la solicitud.
//
// Retorna el refresh token cuyo hash coincide, nil si no se encontró, o un error.
func (r *RefreshTokenRepository) FindByRawToken(ctx context.Context, rawToken string) (*domain.RefreshToken, error) {
	// Obtiene los tokens activos más recientes para comparar con bcrypt.
	// Filtros: is_revoked=FALSE (no revocados) y expires_at>NOW() (no expirados).
	// LIMIT 1000 acota el tiempo de respuesta en el peor caso.
	const q = `
		SELECT id, user_id, app_id, token_hash, device_info, expires_at, used_at, is_revoked, created_at
		FROM refresh_tokens
		WHERE is_revoked = FALSE AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1000`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("refresh_token_repo: find by raw token scan: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t domain.RefreshToken
		var deviceInfoRaw []byte
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.AppID, &t.TokenHash,
			&deviceInfoRaw, &t.ExpiresAt, &t.UsedAt, &t.IsRevoked, &t.CreatedAt,
		); err != nil {
			continue // fila corrupta: ignorar y continuar con la siguiente
		}
		// bcrypt.CompareHashAndPassword devuelve nil si el token raw coincide con el hash almacenado.
		if bcrypt.CompareHashAndPassword([]byte(t.TokenHash), []byte(rawToken)) == nil {
			if len(deviceInfoRaw) > 0 {
				_ = json.Unmarshal(deviceInfoRaw, &t.DeviceInfo)
			}
			return &t, nil
		}
	}
	return nil, rows.Err()
}

// RevokeAllForUserAllApps revoca todos los refresh tokens activos de un usuario en
// todas las aplicaciones. Se usa cuando un administrador desactiva o bloquea un usuario,
// para invalidar inmediatamente todas sus sesiones activas en cualquier aplicación.
// Parámetros:
//   - userID: UUID del usuario cuyos tokens se revocan en todas las apps.
func (r *RefreshTokenRepository) RevokeAllForUserAllApps(ctx context.Context, userID uuid.UUID) error {
	const q = `UPDATE refresh_tokens SET is_revoked = TRUE WHERE user_id = $1 AND is_revoked = FALSE`
	// $1 = user_id; el filtro is_revoked=FALSE evita actualizar filas ya revocadas
	_, err := r.db.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("refresh_token_repo: revoke all for user all apps: %w", err)
	}
	return nil
}
