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

// ApplicationRepository es el repositorio de la tabla "applications".
// Gestiona las aplicaciones cliente que usan Sentinel como proveedor de
// autenticación y autorización. Cada aplicación tiene un slug único (identificador
// legible) y un secret_key que actúa como API key para las llamadas HTTP
// (header X-App-Key). La rotación del secret_key se expone como operación
// independiente para no mezclar responsabilidades con el Update general.
// Utiliza pgxpool.Pool para el pool de conexiones de pgx.
type ApplicationRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe
	logger *slog.Logger  // logger estructurado con component="app_repo"
}

// NewApplicationRepository construye un ApplicationRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewApplicationRepository(db *pgxpool.Pool, log *slog.Logger) *ApplicationRepository {
	return &ApplicationRepository{
		db:     db,
		logger: log.With("component", "app_repo"),
	}
}

// appSelectFields lista las columnas de la tabla "applications" en el orden que
// espera scanApp. Se declara como constante para reutilizarse en todos los SELECT.
const appSelectFields = `id, name, slug, secret_key, is_active, created_at, updated_at`

// scanApp mapea una fila de pgx al dominio domain.Application.
// El orden de los campos debe coincidir exactamente con appSelectFields.
func scanApp(row pgx.Row) (*domain.Application, error) {
	var a domain.Application
	err := row.Scan(
		&a.ID,        // id         — UUID de la aplicación
		&a.Name,      // name       — nombre legible de la aplicación
		&a.Slug,      // slug       — identificador URL-friendly único (e.g. "mi-app")
		&a.SecretKey, // secret_key — API key enviada en el header X-App-Key
		&a.IsActive,  // is_active  — FALSE deshabilita la app sin borrar sus datos
		&a.CreatedAt, // created_at — timestamp de creación
		&a.UpdatedAt, // updated_at — timestamp de última modificación
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// FindBySecretKey busca una aplicación por su secret_key (API key).
// Es el método principal del middleware de validación de X-App-Key: en cada
// request entrante se llama a este método para autenticar la aplicación cliente.
// Parámetros:
//   - secretKey: valor del header X-App-Key enviado por el cliente.
//
// Retorna la aplicación encontrada, nil si no existe, o un error de base de datos.
func (r *ApplicationRepository) FindBySecretKey(ctx context.Context, secretKey string) (*domain.Application, error) {
	q := `SELECT ` + appSelectFields + ` FROM applications WHERE secret_key = $1`
	// $1 = secret_key
	row := r.db.QueryRow(ctx, q, secretKey)
	a, err := scanApp(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("app_repo: find by secret key: %w", err)
	}
	return a, nil
}

// FindBySlug busca una aplicación por su slug único.
// Se usa en el endpoint de mapa de permisos (authz_service) para identificar
// la aplicación a partir del slug que viene en la URL o en el token JWT.
// Parámetros:
//   - slug: identificador URL-friendly de la aplicación (e.g. "erp-interno").
//
// Retorna la aplicación encontrada, nil si no existe, o un error de base de datos.
func (r *ApplicationRepository) FindBySlug(ctx context.Context, slug string) (*domain.Application, error) {
	q := `SELECT ` + appSelectFields + ` FROM applications WHERE slug = $1`
	// $1 = slug
	row := r.db.QueryRow(ctx, q, slug)
	a, err := scanApp(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("app_repo: find by slug: %w", err)
	}
	return a, nil
}

// Create inserta una nueva aplicación en la tabla "applications".
// Los campos created_at y updated_at se establecen con NOW() en la base de datos.
// Retorna un error si el slug ya existe (restricción UNIQUE en la tabla).
// Parámetros:
//   - app: struct con los datos de la nueva aplicación (ID ya generado por la capa de servicio).
func (r *ApplicationRepository) Create(ctx context.Context, app *domain.Application) error {
	const q = `
		INSERT INTO applications (id, name, slug, secret_key, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`
	// $1=id, $2=name, $3=slug, $4=secret_key, $5=is_active
	_, err := r.db.Exec(ctx, q, app.ID, app.Name, app.Slug, app.SecretKey, app.IsActive)
	if err != nil {
		return fmt.Errorf("app_repo: create: %w", err)
	}
	return nil
}

// ExistsAny devuelve true si hay al menos una aplicación registrada en la base de datos.
// Es usado por el bootstrapper para saber si el sistema ya fue inicializado: si no existe
// ninguna aplicación, se ejecuta el proceso de bootstrap que crea la app y el usuario
// administrador iniciales.
func (r *ApplicationRepository) ExistsAny(ctx context.Context) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM applications`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("app_repo: exists any: %w", err)
	}
	return count > 0, nil
}

// ApplicationFilter encapsula los parámetros opcionales para filtrar y paginar
// la lista de aplicaciones. Todos los campos son opcionales.
type ApplicationFilter struct {
	Search   string // búsqueda parcial ILIKE en name y slug (case-insensitive)
	IsActive *bool  // nil = sin filtro; true = solo activas; false = solo inactivas
	Page     int    // número de página (base 1)
	PageSize int    // registros por página; máximo 100
}

// List devuelve una lista paginada de aplicaciones y el total de registros que
// coinciden con el filtro. El WHERE se construye dinámicamente usando "WHERE 1=1"
// como base, de modo que se pueden agregar condiciones AND sin necesidad de saber
// si ya existe una cláusula WHERE.
// Se ejecutan dos consultas: primero COUNT(*) para obtener el total, luego la
// consulta de datos con LIMIT/OFFSET para la paginación.
// Parámetros:
//   - f: criterios de búsqueda, filtros y paginación.
//
// Retorna la lista de aplicaciones, el total de registros y cualquier error.
func (r *ApplicationRepository) List(ctx context.Context, f ApplicationFilter) ([]*domain.Application, int, error) {
	// Valores mínimos para evitar páginas inválidas.
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}

	args := []interface{}{}
	where := "WHERE 1=1" // cláusula base que siempre es verdadera; se añaden condiciones con AND
	i := 1               // índice del próximo parámetro posicional

	// Filtro de búsqueda sobre name y slug; se usa el mismo argumento para ambas columnas.
	if f.Search != "" {
		where += fmt.Sprintf(" AND (name ILIKE $%d OR slug ILIKE $%d)", i, i)
		args = append(args, "%"+f.Search+"%")
		i++
	}
	// Filtro de estado activo/inactivo.
	if f.IsActive != nil {
		where += fmt.Sprintf(" AND is_active = $%d", i)
		args = append(args, *f.IsActive)
		i++
	}

	// Primera consulta: total de registros para calcular total_pages en la API.
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM applications `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("app_repo: list count: %w", err)
	}

	// Segunda consulta: datos paginados. i e i+1 corresponden a LIMIT y OFFSET.
	offset := (f.Page - 1) * f.PageSize
	args = append(args, f.PageSize, offset)
	q := `SELECT ` + appSelectFields + ` FROM applications ` + where +
		fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, i, i+1)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("app_repo: list query: %w", err)
	}
	defer rows.Close()

	var apps []*domain.Application
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("app_repo: list scan: %w", err)
		}
		apps = append(apps, a)
	}
	return apps, total, rows.Err()
}

// FindByID busca una aplicación por su UUID primario.
// Se usa desde handlers de admin cuando se tiene el ID de la aplicación (e.g.
// rutas /admin/applications/:id).
// Parámetros:
//   - id: UUID de la aplicación a buscar.
//
// Retorna la aplicación encontrada, nil si no existe, o un error de base de datos.
func (r *ApplicationRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Application, error) {
	q := `SELECT ` + appSelectFields + ` FROM applications WHERE id = $1`
	// $1 = id (UUID)
	row := r.db.QueryRow(ctx, q, id)
	a, err := scanApp(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("app_repo: find by id: %w", err)
	}
	return a, nil
}

// Update modifica el nombre y el estado activo de una aplicación.
// Usa RETURNING para devolver el registro actualizado sin necesidad de un SELECT
// adicional, reduciendo el número de round-trips a la base de datos.
// No permite cambiar el slug porque es el identificador externo de la aplicación
// y podría romper referencias existentes en tokens JWT.
// Parámetros:
//   - id: UUID de la aplicación a modificar.
//   - name: nuevo nombre legible de la aplicación.
//   - isActive: nuevo estado (true = activa, false = deshabilitada).
//
// Retorna la aplicación con sus valores actualizados, nil si no existe, o un error.
func (r *ApplicationRepository) Update(ctx context.Context, id uuid.UUID, name string, isActive bool) (*domain.Application, error) {
	const q = `
		UPDATE applications SET name = $2, is_active = $3, updated_at = NOW()
		WHERE id = $1
		RETURNING ` + appSelectFields
	// $1=id, $2=name, $3=is_active
	row := r.db.QueryRow(ctx, q, id, name, isActive)
	a, err := scanApp(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("app_repo: update: %w", err)
	}
	return a, nil
}

// RotateSecretKey reemplaza el secret_key de una aplicación por uno nuevo.
// La generación del nuevo valor es responsabilidad de la capa de servicio (que
// produce un token aleatorio criptográficamente seguro). Este método solo persiste
// el cambio y verifica que el registro exista (RowsAffected == 0 indica ID inválido).
// Parámetros:
//   - id: UUID de la aplicación cuya clave se rota.
//   - newKey: nuevo secret_key ya generado por el servicio.
func (r *ApplicationRepository) RotateSecretKey(ctx context.Context, id uuid.UUID, newKey string) error {
	const q = `UPDATE applications SET secret_key = $2, updated_at = NOW() WHERE id = $1`
	// $1=id, $2=secret_key (nuevo valor)
	tag, err := r.db.Exec(ctx, q, id, newKey)
	if err != nil {
		return fmt.Errorf("app_repo: rotate secret key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("app_repo: rotate secret key: application not found")
	}
	return nil
}
