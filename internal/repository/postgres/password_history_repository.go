// Package postgres implementa los repositorios de persistencia de Sentinel usando
// el driver pgx v5. Ver user_repository.go para una descripción completa del paquete.
package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PasswordHistoryRepository es el repositorio de la tabla "password_history".
// Almacena el historial de hashes bcrypt de contraseñas usadas por cada usuario.
// Su único propósito es prevenir la reutilización de contraseñas recientes: cuando
// un usuario quiere cambiar su contraseña, el servicio recupera los últimos N hashes
// y verifica que la nueva contraseña no coincida con ninguno de ellos.
//
// La política actual de Sentinel verifica los últimos 5 hashes (N=5). Este número
// está definido en la capa de servicio (auth_service.go), no en este repositorio.
//
// La tabla no tiene columna updated_at (los registros son inmutables una vez
// insertados) y el ID se genera con gen_random_uuid() directamente en PostgreSQL
// para evitar traer un UUID generado desde Go.
// Utiliza pgxpool.Pool para el pool de conexiones de pgx.
type PasswordHistoryRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe
	logger *slog.Logger  // logger estructurado con component="pwd_history_repo"
}

// NewPasswordHistoryRepository construye un PasswordHistoryRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewPasswordHistoryRepository(db *pgxpool.Pool, log *slog.Logger) *PasswordHistoryRepository {
	return &PasswordHistoryRepository{
		db:     db,
		logger: log.With("component", "pwd_history_repo"),
	}
}

// GetLastN devuelve los últimos n hashes bcrypt de contraseñas del usuario,
// ordenados por fecha de creación descendente (el más reciente primero).
// El caller (auth_service) usa estos hashes para comparar con la nueva contraseña
// propuesta mediante bcrypt.CompareHashAndPassword; si coincide con alguno, rechaza
// el cambio por reutilización de contraseña.
// Parámetros:
//   - userID: UUID del usuario cuyo historial se consulta.
//   - n: número máximo de hashes a recuperar (normalmente 5).
//
// Retorna la lista de hashes bcrypt (puede estar vacía para usuarios nuevos) o un error.
func (r *PasswordHistoryRepository) GetLastN(ctx context.Context, userID uuid.UUID, n int) ([]string, error) {
	const q = `
		SELECT password_hash
		FROM password_history
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2`
	// $1=user_id, $2=n (cantidad de hashes a recuperar)
	// ORDER BY created_at DESC: trae primero los más recientes para comparar contra la nueva contraseña

	rows, err := r.db.Query(ctx, q, userID, n)
	if err != nil {
		return nil, fmt.Errorf("password_history: get last n: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("password_history: scan: %w", err)
		}
		hashes = append(hashes, h)
	}
	return hashes, rows.Err()
}

// Add inserta un nuevo hash bcrypt en el historial de contraseñas del usuario.
// Se llama después de un cambio de contraseña exitoso para registrar el hash
// de la contraseña antigua (la que acaba de ser reemplazada), de modo que no
// pueda reutilizarse en los próximos N cambios.
// El ID del registro se genera con gen_random_uuid() en PostgreSQL para
// simplificar el código Go y garantizar unicidad.
// Parámetros:
//   - userID: UUID del usuario al que pertenece la contraseña.
//   - hash: hash bcrypt de la contraseña que se va a archivar en el historial.
func (r *PasswordHistoryRepository) Add(ctx context.Context, userID uuid.UUID, hash string) error {
	const q = `
		INSERT INTO password_history (id, user_id, password_hash, created_at)
		VALUES (gen_random_uuid(), $1, $2, NOW())`
	// gen_random_uuid(): función nativa de PostgreSQL que genera un UUID v4 aleatorio
	// $1=user_id, $2=password_hash (hash bcrypt de la contraseña archivada)

	if _, err := r.db.Exec(ctx, q, userID, hash); err != nil {
		return fmt.Errorf("password_history: add: %w", err)
	}
	return nil
}
