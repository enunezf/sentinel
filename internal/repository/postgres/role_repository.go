// Package postgres implementa los repositorios de persistencia de Sentinel usando
// el driver pgx v5. Ver user_repository.go para una descripción completa del paquete.
package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enunezf/sentinel/internal/domain"
)

// RoleRepository es el repositorio de la tabla "roles".
// Gestiona la persistencia de roles del sistema RBAC (Role-Based Access Control).
// Un rol pertenece a una aplicación específica (application_id) y puede tener
// permisos asignados a través de la tabla intermedia "role_permissions".
// Los roles con is_system=TRUE son creados durante el bootstrap y no deben
// eliminarse; los roles desactivados (is_active=FALSE) siguen existiendo para
// mantener el historial de asignaciones en user_roles.
// Utiliza pgxpool.Pool para el pool de conexiones de pgx.
type RoleRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe
	logger *slog.Logger  // logger estructurado con component="role_repo"
}

// NewRoleRepository construye un RoleRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewRoleRepository(db *pgxpool.Pool, log *slog.Logger) *RoleRepository {
	return &RoleRepository{
		db:     db,
		logger: log.With("component", "role_repo"),
	}
}

// scanRole mapea una fila de pgx al dominio domain.Role.
// El orden de columnas debe coincidir exactamente con la lista
// "id, application_id, name, description, is_system, is_active, created_at, updated_at"
// que usan todas las consultas de este repositorio.
func scanRole(row pgx.Row) (*domain.Role, error) {
	var r domain.Role
	err := row.Scan(
		&r.ID,            // id             — UUID del rol
		&r.ApplicationID, // application_id — app a la que pertenece el rol
		&r.Name,          // name           — nombre único dentro de la aplicación
		&r.Description,   // description    — descripción opcional del rol
		&r.IsSystem,      // is_system       — TRUE para roles creados por bootstrap (no se pueden borrar)
		&r.IsActive,      // is_active       — FALSE deshabilita el rol sin eliminarlo
		&r.CreatedAt,     // created_at
		&r.UpdatedAt,     // updated_at
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// FindByID busca un rol por su UUID primario.
// Parámetros:
//   - id: UUID del rol a buscar.
//
// Retorna el rol encontrado, nil si no existe, o un error de base de datos.
func (r *RoleRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
	const q = `SELECT id, application_id, name, description, is_system, is_active, created_at, updated_at FROM roles WHERE id = $1`
	// $1 = id (UUID del rol)
	row := r.db.QueryRow(ctx, q, id)
	role, err := scanRole(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("role_repo: find by id: %w", err)
	}
	return role, nil
}

// FindByNameAndApp busca un rol por nombre y aplicación.
// Se usa para validar que no exista un rol duplicado antes de crear uno nuevo,
// ya que el par (name, application_id) tiene restricción UNIQUE en la tabla.
// Parámetros:
//   - name: nombre exacto del rol (búsqueda case-sensitive).
//   - appID: UUID de la aplicación a la que debe pertenecer el rol.
//
// Retorna el rol encontrado, nil si no existe, o un error de base de datos.
func (r *RoleRepository) FindByNameAndApp(ctx context.Context, name string, appID uuid.UUID) (*domain.Role, error) {
	const q = `SELECT id, application_id, name, description, is_system, is_active, created_at, updated_at FROM roles WHERE name = $1 AND application_id = $2`
	// $1=name, $2=application_id
	row := r.db.QueryRow(ctx, q, name, appID)
	role, err := scanRole(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("role_repo: find by name and app: %w", err)
	}
	return role, nil
}

// Create inserta un nuevo rol en la tabla "roles".
// Los campos created_at y updated_at se establecen con NOW() en la base de datos.
// Parámetros:
//   - role: struct con todos los campos del rol a insertar (ID ya generado por el servicio).
func (r *RoleRepository) Create(ctx context.Context, role *domain.Role) error {
	const q = `
		INSERT INTO roles (id, application_id, name, description, is_system, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`
	// $1=id, $2=application_id, $3=name, $4=description, $5=is_system, $6=is_active
	_, err := r.db.Exec(ctx, q, role.ID, role.ApplicationID, role.Name, role.Description, role.IsSystem, role.IsActive)
	if err != nil {
		return fmt.Errorf("role_repo: create: %w", err)
	}
	return nil
}

// Update modifica el nombre y la descripción de un rol.
// No permite cambiar application_id ni is_system para preservar la integridad
// del modelo RBAC. is_active se maneja con Deactivate.
// Parámetros:
//   - role: struct con el ID del rol y los nuevos valores de name y description.
func (r *RoleRepository) Update(ctx context.Context, role *domain.Role) error {
	const q = `
		UPDATE roles SET name = $2, description = $3, updated_at = NOW()
		WHERE id = $1`
	// $1=id, $2=name, $3=description
	_, err := r.db.Exec(ctx, q, role.ID, role.Name, role.Description)
	if err != nil {
		return fmt.Errorf("role_repo: update: %w", err)
	}
	return nil
}

// Deactivate deshabilita un rol estableciendo is_active=FALSE.
// No elimina el registro para conservar el historial de asignaciones pasadas
// en user_roles. Los usuarios con este rol asignado dejan de tenerlo efectivo
// porque la consulta de permisos activos filtra por r.is_active=TRUE.
// Parámetros:
//   - id: UUID del rol a desactivar.
func (r *RoleRepository) Deactivate(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE roles SET is_active = FALSE, updated_at = NOW() WHERE id = $1`
	// $1 = id (UUID del rol)
	_, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("role_repo: deactivate: %w", err)
	}
	return nil
}

// RoleFilter encapsula los parámetros para filtrar y paginar la lista de roles.
type RoleFilter struct {
	ApplicationID *uuid.UUID // nil = todos los roles; valor = solo los de esa aplicación
	Page          int        // número de página (base 1)
	PageSize      int        // registros por página; máximo 100
}

// List devuelve una lista paginada de roles y el total de registros que coinciden
// con el filtro. Si ApplicationID es nil, devuelve roles de todas las aplicaciones.
// Ordena por name ASC para una presentación alfabética consistente.
// Parámetros:
//   - filter: criterios de filtrado y paginación.
//
// Retorna la lista de roles, el total de registros y cualquier error.
func (r *RoleRepository) List(ctx context.Context, filter RoleFilter) ([]*domain.Role, int, error) {
	args := []interface{}{}
	where := ""
	// Filtro opcional por aplicación.
	if filter.ApplicationID != nil {
		where = "WHERE application_id = $1"
		args = append(args, *filter.ApplicationID)
	}

	// Primera consulta: total de registros para calcular total_pages.
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM roles `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("role_repo: list count: %w", err)
	}

	// Segunda consulta: datos paginados. idx e idx+1 corresponden a LIMIT y OFFSET.
	// Si se especificó application_id, idx=2; si no, idx=1.
	idx := len(args) + 1
	dataQ := `SELECT id, application_id, name, description, is_system, is_active, created_at, updated_at FROM roles ` +
		where + fmt.Sprintf(` ORDER BY name ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("role_repo: list query: %w", err)
	}
	defer rows.Close()

	var roles []*domain.Role
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("role_repo: scan: %w", err)
		}
		roles = append(roles, role)
	}
	return roles, total, rows.Err()
}

// GetPermissionsCount devuelve el número de permisos asignados a un rol.
// Se usa en la respuesta del endpoint de detalle de rol para mostrar estadísticas
// sin tener que traer todos los permisos.
// Parámetros:
//   - roleID: UUID del rol.
func (r *RoleRepository) GetPermissionsCount(ctx context.Context, roleID uuid.UUID) (int, error) {
	var count int
	// Cuenta filas en role_permissions donde role_id = roleID.
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM role_permissions WHERE role_id = $1`, roleID).Scan(&count)
	return count, err
}

// GetUsersCount devuelve el número de usuarios activamente asignados a un rol.
// Solo cuenta asignaciones activas (is_active=TRUE) en user_roles.
// Parámetros:
//   - roleID: UUID del rol.
func (r *RoleRepository) GetUsersCount(ctx context.Context, roleID uuid.UUID) (int, error) {
	var count int
	// Filtra is_active=TRUE para excluir asignaciones revocadas.
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM user_roles WHERE role_id = $1 AND is_active = TRUE`, roleID).Scan(&count)
	return count, err
}

// GetPermissions devuelve todos los permisos asignados a un rol.
// Realiza un JOIN entre permissions y role_permissions para obtener los datos
// completos de cada permiso en una sola consulta. COALESCE convierte description
// NULL a cadena vacía para evitar NULLs en el struct de dominio.
// Parámetros:
//   - roleID: UUID del rol cuyos permisos se quieren obtener.
//
// Retorna la lista de permisos (puede estar vacía) o un error de base de datos.
func (r *RoleRepository) GetPermissions(ctx context.Context, roleID uuid.UUID) ([]domain.Permission, error) {
	const q = `
		SELECT p.id, p.application_id, p.code, COALESCE(p.description,''), p.scope_type, p.created_at
		FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id
		WHERE rp.role_id = $1`
	// $1 = role_id
	// JOIN: permissions p ← role_permissions rp (tabla intermedia M:N entre roles y permissions)
	rows, err := r.db.Query(ctx, q, roleID)
	if err != nil {
		return nil, fmt.Errorf("role_repo: get permissions: %w", err)
	}
	defer rows.Close()

	var perms []domain.Permission
	for rows.Next() {
		var p domain.Permission
		if err := rows.Scan(&p.ID, &p.ApplicationID, &p.Code, &p.Description, &p.ScopeType, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("role_repo: scan permission: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// AddPermission asigna un permiso a un rol insertando una fila en role_permissions.
// Usa ON CONFLICT DO NOTHING para que la operación sea idempotente: si el permiso
// ya está asignado al rol, la consulta no falla sino que no hace nada.
// Parámetros:
//   - roleID: UUID del rol al que se asigna el permiso.
//   - permissionID: UUID del permiso a asignar.
func (r *RoleRepository) AddPermission(ctx context.Context, roleID, permissionID uuid.UUID) error {
	const q = `INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	// $1=role_id, $2=permission_id
	_, err := r.db.Exec(ctx, q, roleID, permissionID)
	if err != nil {
		return fmt.Errorf("role_repo: add permission: %w", err)
	}
	return nil
}

// RemovePermission elimina la asignación de un permiso a un rol borrando la fila
// correspondiente de role_permissions.
// Parámetros:
//   - roleID: UUID del rol del que se retira el permiso.
//   - permissionID: UUID del permiso a retirar.
func (r *RoleRepository) RemovePermission(ctx context.Context, roleID, permissionID uuid.UUID) error {
	const q = `DELETE FROM role_permissions WHERE role_id = $1 AND permission_id = $2`
	// $1=role_id, $2=permission_id
	_, err := r.db.Exec(ctx, q, roleID, permissionID)
	if err != nil {
		return fmt.Errorf("role_repo: remove permission: %w", err)
	}
	return nil
}

// GetRolesForPermission devuelve los nombres de todos los roles activos que tienen
// asignado un permiso determinado. Se usa en la respuesta del endpoint de detalle
// de permiso para mostrar qué roles lo otorgan.
// Solo devuelve roles con is_active=TRUE para excluir roles desactivados.
// Parámetros:
//   - permissionID: UUID del permiso.
//
// Retorna una lista de nombres de roles o un error de base de datos.
func (r *RoleRepository) GetRolesForPermission(ctx context.Context, permissionID uuid.UUID) ([]string, error) {
	const q = `
		SELECT r.name
		FROM role_permissions rp
		JOIN roles r ON r.id = rp.role_id
		WHERE rp.permission_id = $1 AND r.is_active = TRUE`
	// $1 = permission_id
	// JOIN: role_permissions rp → roles r (para obtener el nombre del rol)
	rows, err := r.db.Query(ctx, q, permissionID)
	if err != nil {
		return nil, fmt.Errorf("role_repo: get roles for permission: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("role_repo: scan role name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
