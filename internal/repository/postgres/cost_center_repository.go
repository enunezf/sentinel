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

// CostCenterRepository es el repositorio de la tabla "cost_centers".
// Un centro de costo (cost center) representa una unidad organizacional o
// presupuestaria dentro de una aplicación. Se usa para implementar autorización
// de alcance restringido: cuando un permiso tiene scope_type="cost_center", el
// usuario solo puede ejercerlo sobre los cost centers que tiene asignados en la
// tabla user_cost_centers.
// Cada cost center pertenece a una aplicación (application_id) y se identifica
// por un código alfanumérico único dentro de esa aplicación (e.g. "CC-001").
// Utiliza pgxpool.Pool para el pool de conexiones de pgx.
type CostCenterRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe
	logger *slog.Logger  // logger estructurado con component="cc_repo"
}

// NewCostCenterRepository construye un CostCenterRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewCostCenterRepository(db *pgxpool.Pool, log *slog.Logger) *CostCenterRepository {
	return &CostCenterRepository{
		db:     db,
		logger: log.With("component", "cc_repo"),
	}
}

// scanCostCenter mapea una fila de pgx al dominio domain.CostCenter.
// Todas las consultas de este repositorio seleccionan las columnas en el orden:
// id, application_id, code, name, is_active, created_at.
func scanCostCenter(row pgx.Row) (*domain.CostCenter, error) {
	var cc domain.CostCenter
	err := row.Scan(
		&cc.ID,            // id             — UUID del cost center
		&cc.ApplicationID, // application_id — app a la que pertenece el cost center
		&cc.Code,          // code           — código único dentro de la app (e.g. "CC-001")
		&cc.Name,          // name           — nombre descriptivo del cost center
		&cc.IsActive,      // is_active       — FALSE deshabilita el cost center sin eliminarlo
		&cc.CreatedAt,     // created_at     — timestamp de creación (no hay updated_at en esta tabla)
	)
	if err != nil {
		return nil, err
	}
	return &cc, nil
}

// FindByID busca un cost center por su UUID primario.
// Parámetros:
//   - id: UUID del cost center a buscar.
//
// Retorna el cost center encontrado, nil si no existe, o un error de base de datos.
func (r *CostCenterRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.CostCenter, error) {
	const q = `SELECT id, application_id, code, name, is_active, created_at FROM cost_centers WHERE id = $1`
	// $1 = id (UUID del cost center)
	cc, err := scanCostCenter(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("cc_repo: find by id: %w", err)
	}
	return cc, nil
}

// Create inserta un nuevo cost center en la tabla "cost_centers".
// created_at se establece con NOW() en la base de datos.
// Retorna un error si el par (application_id, code) ya existe (restricción UNIQUE).
// Parámetros:
//   - cc: struct con todos los campos del cost center (ID ya generado por el servicio).
func (r *CostCenterRepository) Create(ctx context.Context, cc *domain.CostCenter) error {
	const q = `
		INSERT INTO cost_centers (id, application_id, code, name, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())`
	// $1=id, $2=application_id, $3=code, $4=name, $5=is_active
	_, err := r.db.Exec(ctx, q, cc.ID, cc.ApplicationID, cc.Code, cc.Name, cc.IsActive)
	if err != nil {
		return fmt.Errorf("cc_repo: create: %w", err)
	}
	return nil
}

// Update modifica el nombre y el estado is_active de un cost center.
// El campo code no se puede cambiar porque es el identificador externo del
// cost center (puede estar referenciado en JWT claims y reportes).
// Parámetros:
//   - cc: struct con el ID del cost center y los nuevos valores de name e is_active.
func (r *CostCenterRepository) Update(ctx context.Context, cc *domain.CostCenter) error {
	const q = `UPDATE cost_centers SET name = $2, is_active = $3 WHERE id = $1`
	// $1=id, $2=name, $3=is_active
	_, err := r.db.Exec(ctx, q, cc.ID, cc.Name, cc.IsActive)
	if err != nil {
		return fmt.Errorf("cc_repo: update: %w", err)
	}
	return nil
}

// CCFilter encapsula los parámetros para filtrar y paginar la lista de cost centers.
type CCFilter struct {
	ApplicationID *uuid.UUID // nil = todos los cost centers; valor = solo los de esa aplicación
	Page          int        // número de página (base 1)
	PageSize      int        // registros por página; máximo 100
}

// List devuelve una lista paginada de cost centers y el total de registros que
// coinciden con el filtro. Si ApplicationID es nil, devuelve cost centers de todas
// las aplicaciones. Ordena por code ASC para una presentación consistente.
// Se ejecutan dos consultas: primero COUNT(*) para el total, luego datos con LIMIT/OFFSET.
// Parámetros:
//   - filter: criterios de filtrado y paginación.
//
// Retorna la lista de cost centers (nunca nil, puede ser slice vacío), el total y cualquier error.
func (r *CostCenterRepository) List(ctx context.Context, filter CCFilter) ([]*domain.CostCenter, int, error) {
	args := []interface{}{}
	where := ""
	// Filtro opcional por aplicación.
	if filter.ApplicationID != nil {
		where = "WHERE application_id = $1"
		args = append(args, *filter.ApplicationID)
	}

	// Primera consulta: total de registros.
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM cost_centers `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("cc_repo: list count: %w", err)
	}

	// Segunda consulta: datos paginados. idx e idx+1 corresponden a LIMIT y OFFSET.
	idx := len(args) + 1
	dataQ := `SELECT id, application_id, code, name, is_active, created_at FROM cost_centers ` +
		where + fmt.Sprintf(` ORDER BY code ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("cc_repo: list query: %w", err)
	}
	defer rows.Close()

	// make inicializa un slice vacío (no nil) para que el marshaling a JSON
	// devuelva [] en lugar de null cuando no hay resultados.
	ccs := make([]*domain.CostCenter, 0)
	for rows.Next() {
		cc, err := scanCostCenter(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("cc_repo: scan: %w", err)
		}
		ccs = append(ccs, cc)
	}
	return ccs, total, rows.Err()
}

// ListByApp devuelve todos los cost centers de una aplicación sin paginación.
// Se usa en el servicio de autorización para construir el contexto completo de
// un usuario (qué cost centers tiene asignados) al generar el JWT o al consultar
// permisos. Ordena por code ASC para presentación consistente.
// Parámetros:
//   - appID: UUID de la aplicación cuyos cost centers se quieren obtener.
//
// Retorna la lista completa de cost centers o un error de base de datos.
func (r *CostCenterRepository) ListByApp(ctx context.Context, appID uuid.UUID) ([]*domain.CostCenter, error) {
	const q = `SELECT id, application_id, code, name, is_active, created_at FROM cost_centers WHERE application_id = $1 ORDER BY code ASC`
	// $1 = application_id
	rows, err := r.db.Query(ctx, q, appID)
	if err != nil {
		return nil, fmt.Errorf("cc_repo: list by app: %w", err)
	}
	defer rows.Close()

	var ccs []*domain.CostCenter
	for rows.Next() {
		cc, err := scanCostCenter(rows)
		if err != nil {
			return nil, fmt.Errorf("cc_repo: scan: %w", err)
		}
		ccs = append(ccs, cc)
	}
	return ccs, rows.Err()
}
