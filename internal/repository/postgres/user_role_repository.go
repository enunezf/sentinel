// Package postgres implementa los repositorios de persistencia de Sentinel usando
// el driver pgx v5. Ver user_repository.go para una descripción completa del paquete.
package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enunezf/sentinel/internal/domain"
)

// UserRoleRepository es el repositorio de la tabla "user_roles".
// La tabla user_roles implementa la asignación M:N entre usuarios y roles dentro
// de una aplicación específica. Una asignación de rol puede tener una vigencia
// temporal (valid_from, valid_until) y puede ser revocada sin eliminar el registro
// histórico (is_active=FALSE). El campo granted_by registra qué usuario (normalmente
// un administrador) realizó la asignación.
// Utiliza pgxpool.Pool para el pool de conexiones de pgx.
type UserRoleRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe
	logger *slog.Logger  // logger estructurado con component="user_role_repo"
}

// NewUserRoleRepository construye un UserRoleRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewUserRoleRepository(db *pgxpool.Pool, log *slog.Logger) *UserRoleRepository {
	return &UserRoleRepository{
		db:     db,
		logger: log.With("component", "user_role_repo"),
	}
}

// Assign crea una nueva asignación de rol a un usuario en la tabla "user_roles".
// is_active se inicializa en TRUE y created_at con NOW().
// valid_until puede ser NULL si la asignación no tiene fecha de expiración.
// Parámetros:
//   - ur: struct domain.UserRole con todos los campos necesarios (ID ya generado por el servicio).
func (r *UserRoleRepository) Assign(ctx context.Context, ur *domain.UserRole) error {
	const q = `
		INSERT INTO user_roles (id, user_id, role_id, application_id, granted_by, valid_from, valid_until, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE, NOW())`
	// $1=id, $2=user_id, $3=role_id, $4=application_id
	// $5=granted_by (UUID del admin que asignó el rol), $6=valid_from, $7=valid_until (nullable)
	_, err := r.db.Exec(ctx, q,
		ur.ID, ur.UserID, ur.RoleID, ur.ApplicationID,
		ur.GrantedBy, ur.ValidFrom, ur.ValidUntil,
	)
	if err != nil {
		return fmt.Errorf("user_role_repo: assign: %w", err)
	}
	return nil
}

// Revoke desactiva una asignación de rol estableciendo is_active=FALSE.
// No elimina el registro para preservar el historial de auditoría. Después de
// revocar, el rol ya no aparecerá en el JWT del usuario ni en las verificaciones
// de permiso porque las consultas de permisos activos filtran por is_active=TRUE.
// Parámetros:
//   - id: UUID de la asignación user_role a revocar.
func (r *UserRoleRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE user_roles SET is_active = FALSE WHERE id = $1`
	// $1 = id (UUID de la asignación)
	_, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("user_role_repo: revoke: %w", err)
	}
	return nil
}

// FindByID busca una asignación user_role por su UUID primario.
// Realiza un JOIN con la tabla "roles" para incluir el nombre del rol (r.name)
// en el resultado, evitando un segundo SELECT. Esto es necesario porque el struct
// domain.UserRole incluye el campo RoleName para facilitar la presentación en la API.
// Parámetros:
//   - id: UUID de la asignación a buscar.
//
// Retorna la asignación encontrada o un error (no devuelve nil como los otros Find,
// porque se espera que el caller ya validó la existencia).
func (r *UserRoleRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.UserRole, error) {
	const q = `
		SELECT ur.id, ur.user_id, ur.role_id, ur.application_id, ur.granted_by,
		       ur.valid_from, ur.valid_until, ur.is_active, ur.created_at, r.name
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.id = $1`
	// $1 = id (UUID de la asignación)
	// JOIN: user_roles ur → roles r (para obtener el nombre del rol sin un SELECT adicional)
	row := r.db.QueryRow(ctx, q, id)
	var ur domain.UserRole
	err := row.Scan(&ur.ID, &ur.UserID, &ur.RoleID, &ur.ApplicationID, &ur.GrantedBy,
		&ur.ValidFrom, &ur.ValidUntil, &ur.IsActive, &ur.CreatedAt, &ur.RoleName)
	if err != nil {
		return nil, fmt.Errorf("user_role_repo: find by id: %w", err)
	}
	return &ur, nil
}

// ListForUser devuelve todas las asignaciones de roles de un usuario en todas
// las aplicaciones, incluyendo las revocadas e inactivas (para historial).
// Realiza un JOIN con "roles" para incluir el nombre del rol en cada asignación.
// Ordena por created_at DESC para mostrar las asignaciones más recientes primero.
// Parámetros:
//   - userID: UUID del usuario cuyas asignaciones se quieren listar.
//
// Retorna la lista de asignaciones (puede estar vacía) o un error de base de datos.
func (r *UserRoleRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]*domain.UserRole, error) {
	const q = `
		SELECT ur.id, ur.user_id, ur.role_id, ur.application_id, ur.granted_by,
		       ur.valid_from, ur.valid_until, ur.is_active, ur.created_at, r.name
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY ur.created_at DESC`
	// $1 = user_id
	// JOIN: user_roles ur → roles r (para obtener el nombre del rol)
	rows, err := r.db.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("user_role_repo: list for user: %w", err)
	}
	defer rows.Close()

	var result []*domain.UserRole
	for rows.Next() {
		var ur domain.UserRole
		if err := rows.Scan(&ur.ID, &ur.UserID, &ur.RoleID, &ur.ApplicationID, &ur.GrantedBy,
			&ur.ValidFrom, &ur.ValidUntil, &ur.IsActive, &ur.CreatedAt, &ur.RoleName); err != nil {
			return nil, fmt.Errorf("user_role_repo: scan: %w", err)
		}
		result = append(result, &ur)
	}
	return result, rows.Err()
}

// GetActiveRoleNamesForUserApp devuelve los nombres de los roles activos de un usuario
// en una aplicación específica. Este método es crítico para la generación del JWT:
// los roles activos se incluyen en el claim "roles" del token de acceso.
//
// Una asignación se considera activa si cumple todas estas condiciones:
//   - ur.is_active = TRUE (no fue revocada manualmente)
//   - r.is_active  = TRUE (el rol en sí no fue desactivado)
//   - ur.valid_from <= NOW() (la asignación ya entró en vigencia)
//   - ur.valid_until IS NULL OR ur.valid_until > NOW() (no ha expirado)
//
// Parámetros:
//   - userID: UUID del usuario.
//   - appID:  UUID de la aplicación.
//
// Retorna la lista de nombres de roles activos o un error de base de datos.
func (r *UserRoleRepository) GetActiveRoleNamesForUserApp(ctx context.Context, userID, appID uuid.UUID) ([]string, error) {
	const q = `
		SELECT r.name
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		  AND ur.application_id = $2
		  AND ur.is_active = TRUE
		  AND r.is_active = TRUE
		  AND ur.valid_from <= NOW()
		  AND (ur.valid_until IS NULL OR ur.valid_until > NOW())`
	// $1=user_id, $2=application_id
	// JOIN: user_roles ur → roles r (para filtrar por r.is_active y obtener r.name)
	rows, err := r.db.Query(ctx, q, userID, appID)
	if err != nil {
		return nil, fmt.Errorf("user_role_repo: get active role names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("user_role_repo: scan role name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
