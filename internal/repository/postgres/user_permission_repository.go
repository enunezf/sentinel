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

// UserPermissionRepository es el repositorio de la tabla "user_permissions".
// La tabla user_permissions permite asignar permisos directamente a un usuario,
// sin necesidad de pasar por un rol. Esto implementa el "permiso directo" en RBAC:
// además de los permisos que hereda de sus roles, un usuario puede tener permisos
// adicionales asignados individualmente.
// Al igual que user_roles, las asignaciones tienen vigencia temporal (valid_from,
// valid_until), pueden ser revocadas (is_active=FALSE) y registran quién las otorgó
// (granted_by).
// Utiliza pgxpool.Pool para el pool de conexiones de pgx.
type UserPermissionRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe
	logger *slog.Logger  // logger estructurado con component="user_perm_repo"
}

// NewUserPermissionRepository construye un UserPermissionRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewUserPermissionRepository(db *pgxpool.Pool, log *slog.Logger) *UserPermissionRepository {
	return &UserPermissionRepository{
		db:     db,
		logger: log.With("component", "user_perm_repo"),
	}
}

// Assign crea una nueva asignación directa de permiso a un usuario en la tabla "user_permissions".
// is_active se inicializa en TRUE y created_at con NOW().
// valid_until puede ser NULL si la asignación no tiene fecha de expiración.
// Parámetros:
//   - up: struct domain.UserPermission con todos los campos necesarios (ID ya generado por el servicio).
func (r *UserPermissionRepository) Assign(ctx context.Context, up *domain.UserPermission) error {
	const q = `
		INSERT INTO user_permissions (id, user_id, permission_id, application_id, granted_by, valid_from, valid_until, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE, NOW())`
	// $1=id, $2=user_id, $3=permission_id, $4=application_id
	// $5=granted_by (UUID del admin que asignó el permiso), $6=valid_from, $7=valid_until (nullable)
	_, err := r.db.Exec(ctx, q,
		up.ID, up.UserID, up.PermissionID, up.ApplicationID,
		up.GrantedBy, up.ValidFrom, up.ValidUntil,
	)
	if err != nil {
		return fmt.Errorf("user_perm_repo: assign: %w", err)
	}
	return nil
}

// Revoke desactiva una asignación directa de permiso estableciendo is_active=FALSE.
// No elimina el registro para preservar el historial de auditoría. Después de
// revocar, el permiso ya no será efectivo porque las consultas de permisos activos
// filtran por is_active=TRUE.
// Parámetros:
//   - id: UUID de la asignación user_permission a revocar.
func (r *UserPermissionRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE user_permissions SET is_active = FALSE WHERE id = $1`
	// $1 = id (UUID de la asignación)
	_, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("user_perm_repo: revoke: %w", err)
	}
	return nil
}

// FindByID busca una asignación user_permission por su UUID primario.
// Realiza un JOIN con "permissions" para incluir el código del permiso (p.code)
// en el resultado, evitando un segundo SELECT. El campo PermissionCode del struct
// se usa en la respuesta de la API.
// Parámetros:
//   - id: UUID de la asignación a buscar.
//
// Retorna la asignación encontrada o un error de base de datos.
func (r *UserPermissionRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.UserPermission, error) {
	const q = `
		SELECT up.id, up.user_id, up.permission_id, up.application_id, up.granted_by,
		       up.valid_from, up.valid_until, up.is_active, up.created_at, p.code
		FROM user_permissions up
		JOIN permissions p ON p.id = up.permission_id
		WHERE up.id = $1`
	// $1 = id (UUID de la asignación)
	// JOIN: user_permissions up → permissions p (para obtener p.code sin SELECT adicional)
	row := r.db.QueryRow(ctx, q, id)
	var up domain.UserPermission
	err := row.Scan(&up.ID, &up.UserID, &up.PermissionID, &up.ApplicationID, &up.GrantedBy,
		&up.ValidFrom, &up.ValidUntil, &up.IsActive, &up.CreatedAt, &up.PermissionCode)
	if err != nil {
		return nil, fmt.Errorf("user_perm_repo: find by id: %w", err)
	}
	return &up, nil
}

// ListForUser devuelve todas las asignaciones directas de permisos de un usuario en
// todas las aplicaciones, incluyendo las revocadas e inactivas (para historial completo).
// Realiza un JOIN con "permissions" para incluir el código del permiso en cada asignación.
// Ordena por created_at DESC para mostrar las asignaciones más recientes primero.
// Parámetros:
//   - userID: UUID del usuario cuyas asignaciones se quieren listar.
//
// Retorna la lista de asignaciones (puede estar vacía) o un error de base de datos.
func (r *UserPermissionRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]*domain.UserPermission, error) {
	const q = `
		SELECT up.id, up.user_id, up.permission_id, up.application_id, up.granted_by,
		       up.valid_from, up.valid_until, up.is_active, up.created_at, p.code
		FROM user_permissions up
		JOIN permissions p ON p.id = up.permission_id
		WHERE up.user_id = $1
		ORDER BY up.created_at DESC`
	// $1 = user_id
	// JOIN: user_permissions up → permissions p (para obtener p.code)
	rows, err := r.db.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("user_perm_repo: list for user: %w", err)
	}
	defer rows.Close()

	var result []*domain.UserPermission
	for rows.Next() {
		var up domain.UserPermission
		if err := rows.Scan(&up.ID, &up.UserID, &up.PermissionID, &up.ApplicationID, &up.GrantedBy,
			&up.ValidFrom, &up.ValidUntil, &up.IsActive, &up.CreatedAt, &up.PermissionCode); err != nil {
			return nil, fmt.Errorf("user_perm_repo: scan: %w", err)
		}
		result = append(result, &up)
	}
	return result, rows.Err()
}

// GetActivePermissionCodesForUserApp devuelve los códigos de los permisos directos
// activos de un usuario en una aplicación específica. Es crítico para la generación
// del JWT y la verificación de autorización: estos códigos se incluyen junto con los
// permisos heredados de roles en el claim "permissions" del token.
//
// DISTINCT evita duplicados en caso de que el mismo permiso haya sido asignado
// varias veces (e.g. se revocó y se volvió a asignar).
//
// Una asignación se considera activa si cumple todas estas condiciones:
//   - up.is_active = TRUE (no fue revocada manualmente)
//   - up.valid_from <= NOW() (la asignación ya entró en vigencia)
//   - up.valid_until IS NULL OR up.valid_until > NOW() (no ha expirado)
//
// Parámetros:
//   - userID: UUID del usuario.
//   - appID:  UUID de la aplicación.
//
// Retorna la lista de códigos de permisos activos o un error de base de datos.
func (r *UserPermissionRepository) GetActivePermissionCodesForUserApp(ctx context.Context, userID, appID uuid.UUID) ([]string, error) {
	const q = `
		SELECT DISTINCT p.code
		FROM user_permissions up
		JOIN permissions p ON p.id = up.permission_id
		WHERE up.user_id = $1
		  AND up.application_id = $2
		  AND up.is_active = TRUE
		  AND up.valid_from <= NOW()
		  AND (up.valid_until IS NULL OR up.valid_until > NOW())`
	// $1=user_id, $2=application_id
	// DISTINCT: elimina duplicados si el mismo permiso fue asignado múltiples veces
	// JOIN: user_permissions up → permissions p (para obtener el código legible del permiso)
	rows, err := r.db.Query(ctx, q, userID, appID)
	if err != nil {
		return nil, fmt.Errorf("user_perm_repo: get active codes: %w", err)
	}
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("user_perm_repo: scan code: %w", err)
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}
