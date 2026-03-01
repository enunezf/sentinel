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

// PermissionRepository es el repositorio de la tabla "permissions".
// Gestiona la persistencia de los permisos del sistema RBAC. Un permiso representa
// una acción autorizable sobre un recurso (e.g. "invoices:read", "users:write") y
// pertenece a una aplicación específica (application_id). El campo scope_type
// determina si el permiso se aplica a nivel global o por cost_center.
// Cuando se elimina un permiso, la base de datos elimina en cascada las filas
// correspondientes en role_permissions y user_permissions (FK con ON DELETE CASCADE).
// Utiliza pgxpool.Pool para el pool de conexiones de pgx.
type PermissionRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe
	logger *slog.Logger  // logger estructurado con component="perm_repo"
}

// NewPermissionRepository construye un PermissionRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewPermissionRepository(db *pgxpool.Pool, log *slog.Logger) *PermissionRepository {
	return &PermissionRepository{
		db:     db,
		logger: log.With("component", "perm_repo"),
	}
}

// scanPermission mapea una fila de pgx al dominio domain.Permission.
// Todas las consultas de este repositorio seleccionan las columnas en el orden:
// id, application_id, code, description (con COALESCE para evitar NULL), scope_type, created_at.
func scanPermission(row pgx.Row) (*domain.Permission, error) {
	var p domain.Permission
	err := row.Scan(
		&p.ID,            // id             — UUID del permiso
		&p.ApplicationID, // application_id — app a la que pertenece el permiso
		&p.Code,          // code           — código único dentro de la app (e.g. "invoices:read")
		&p.Description,   // description    — descripción legible del permiso (COALESCE → vacío si NULL)
		&p.ScopeType,     // scope_type     — enum: "global" o "cost_center"
		&p.CreatedAt,     // created_at     — timestamp de creación (no hay updated_at en esta tabla)
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// FindByID busca un permiso por su UUID primario.
// COALESCE(description,'') convierte valores NULL a cadena vacía para evitar
// que el campo Description del struct quede como nil/vacío en Go.
// Parámetros:
//   - id: UUID del permiso a buscar.
//
// Retorna el permiso encontrado, nil si no existe, o un error de base de datos.
func (r *PermissionRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Permission, error) {
	const q = `SELECT id, application_id, code, COALESCE(description,''), scope_type, created_at FROM permissions WHERE id = $1`
	// $1 = id (UUID del permiso)
	p, err := scanPermission(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("perm_repo: find by id: %w", err)
	}
	return p, nil
}

// FindByCodeAndApp busca un permiso por código y aplicación.
// Se usa para validar unicidad antes de crear un permiso nuevo, ya que el par
// (code, application_id) tiene restricción UNIQUE en la tabla.
// Parámetros:
//   - code: código exacto del permiso (e.g. "invoices:read"); búsqueda case-sensitive.
//   - appID: UUID de la aplicación a la que debe pertenecer el permiso.
//
// Retorna el permiso encontrado, nil si no existe, o un error de base de datos.
func (r *PermissionRepository) FindByCodeAndApp(ctx context.Context, code string, appID uuid.UUID) (*domain.Permission, error) {
	const q = `SELECT id, application_id, code, COALESCE(description,''), scope_type, created_at FROM permissions WHERE code = $1 AND application_id = $2`
	// $1=code, $2=application_id
	p, err := scanPermission(r.db.QueryRow(ctx, q, code, appID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("perm_repo: find by code and app: %w", err)
	}
	return p, nil
}

// Create inserta un nuevo permiso en la tabla "permissions".
// La tabla "permissions" no tiene columna updated_at; solo tiene created_at que
// se establece con NOW(). scope_type se pasa como string para que PostgreSQL lo
// convierta al tipo enum correspondiente.
// Parámetros:
//   - p: struct con todos los campos del permiso (ID ya generado por el servicio).
func (r *PermissionRepository) Create(ctx context.Context, p *domain.Permission) error {
	const q = `
		INSERT INTO permissions (id, application_id, code, description, scope_type, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())`
	// $1=id, $2=application_id, $3=code, $4=description, $5=scope_type (string→enum PostgreSQL)
	_, err := r.db.Exec(ctx, q, p.ID, p.ApplicationID, p.Code, p.Description, string(p.ScopeType))
	if err != nil {
		return fmt.Errorf("perm_repo: create: %w", err)
	}
	return nil
}

// Delete elimina un permiso de la tabla "permissions" por su ID.
// Gracias a las restricciones de clave foránea con ON DELETE CASCADE definidas
// en el esquema, la eliminación del permiso arrastra automáticamente:
//   - las filas de role_permissions que lo referencian
//   - las filas de user_permissions que lo referencian
//
// Parámetros:
//   - id: UUID del permiso a eliminar.
func (r *PermissionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM permissions WHERE id = $1`
	// $1 = id (UUID del permiso)
	_, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("perm_repo: delete: %w", err)
	}
	return nil
}

// PermissionFilter encapsula los parámetros para filtrar y paginar la lista de permisos.
type PermissionFilter struct {
	ApplicationID *uuid.UUID // nil = todos los permisos; valor = solo los de esa aplicación
	Page          int        // número de página (base 1)
	PageSize      int        // registros por página; máximo 100
}

// List devuelve una lista paginada de permisos y el total de registros que coinciden
// con el filtro. Si ApplicationID es nil, devuelve permisos de todas las aplicaciones.
// Ordena por code ASC para una presentación alfabética consistente.
// Se ejecutan dos consultas: primero COUNT(*) para el total, luego datos con LIMIT/OFFSET.
// Parámetros:
//   - filter: criterios de filtrado y paginación.
//
// Retorna la lista de permisos (nunca nil, puede ser slice vacío), el total y cualquier error.
func (r *PermissionRepository) List(ctx context.Context, filter PermissionFilter) ([]*domain.Permission, int, error) {
	args := []interface{}{}
	where := ""
	// Filtro opcional por aplicación.
	if filter.ApplicationID != nil {
		where = "WHERE application_id = $1"
		args = append(args, *filter.ApplicationID)
	}

	// Primera consulta: total de registros.
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM permissions `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("perm_repo: list count: %w", err)
	}

	// Segunda consulta: datos paginados. idx e idx+1 corresponden a LIMIT y OFFSET.
	idx := len(args) + 1
	dataQ := `SELECT id, application_id, code, COALESCE(description,''), scope_type, created_at FROM permissions ` +
		where + fmt.Sprintf(` ORDER BY code ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("perm_repo: list query: %w", err)
	}
	defer rows.Close()

	// make inicializa un slice vacío (no nil) para que el marshaling a JSON
	// devuelva [] en lugar de null cuando no hay resultados.
	perms := make([]*domain.Permission, 0)
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("perm_repo: scan: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, total, rows.Err()
}

// ListByApp devuelve todos los permisos de una aplicación sin paginación.
// Se usa en el servicio de autorización (authz_service) para construir el mapa
// completo de permisos de la aplicación que se firma con RSA-SHA256 y se cachea
// en Redis. El orden code ASC garantiza que el JSON canónico sea determinista.
// Parámetros:
//   - appID: UUID de la aplicación cuyos permisos se quieren obtener.
//
// Retorna la lista completa de permisos o un error de base de datos.
func (r *PermissionRepository) ListByApp(ctx context.Context, appID uuid.UUID) ([]*domain.Permission, error) {
	const q = `SELECT id, application_id, code, COALESCE(description,''), scope_type, created_at FROM permissions WHERE application_id = $1 ORDER BY code ASC`
	// $1 = application_id
	rows, err := r.db.Query(ctx, q, appID)
	if err != nil {
		return nil, fmt.Errorf("perm_repo: list by app: %w", err)
	}
	defer rows.Close()

	var perms []*domain.Permission
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, fmt.Errorf("perm_repo: scan: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}
