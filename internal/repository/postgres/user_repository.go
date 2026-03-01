// Package postgres implementa los repositorios de persistencia de Sentinel usando
// el driver pgx v5 para comunicarse con PostgreSQL. Cada repositorio en este paquete
// corresponde a una tabla (o conjunto de tablas relacionadas) del esquema de la base
// de datos. Todas las consultas usan parámetros posicionales ($1, $2, …) para prevenir
// inyección SQL.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enunezf/sentinel/internal/domain"
)

// UserRepository es el repositorio de la tabla "users".
// Gestiona la persistencia de usuarios del sistema, incluyendo sus credenciales
// (hash bcrypt de la contraseña), estado de cuenta, controles de bloqueo por
// intentos fallidos de login y metadatos de auditoría.
// Utiliza pgxpool.Pool para aprovechar el pool de conexiones de pgx.
type UserRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe, compartido por toda la aplicación
	logger *slog.Logger  // logger estructurado con el atributo component="user_repo" para filtrado de logs
}

// NewUserRepository construye un UserRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación (se le añade el atributo component="user_repo" internamente).
// Devuelve un puntero al repositorio inicializado.
func NewUserRepository(db *pgxpool.Pool, log *slog.Logger) *UserRepository {
	return &UserRepository{
		db:     db,
		logger: log.With("component", "user_repo"),
	}
}

// scanUser mapea una fila de pgx (pgx.Row o pgx.Rows) al dominio domain.User.
// El orden de los campos debe coincidir exactamente con la constante userSelectFields.
// lockoutDate se escanea a una variable auxiliar *time.Time porque el campo puede
// ser NULL en la base de datos; luego se asigna al struct.
func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	var lockoutDate *time.Time // auxiliar para campo nullable lockout_date
	err := row.Scan(
		&u.ID,             // id          — UUID del usuario
		&u.Username,       // username    — nombre de usuario único
		&u.Email,          // email       — dirección de correo único
		&u.PasswordHash,   // password_hash — hash bcrypt (costo ≥ 12) de la contraseña
		&u.IsActive,       // is_active   — FALSE deshabilita el login sin borrar el registro
		&u.MustChangePwd,  // must_change_pwd — TRUE obliga al usuario a cambiar contraseña en el próximo login
		&u.LastLoginAt,    // last_login_at — timestamp del último login exitoso (nullable)
		&u.FailedAttempts, // failed_attempts — contador de intentos fallidos consecutivos
		&u.LockedUntil,    // locked_until — timestamp hasta el que la cuenta está bloqueada (nullable)
		&u.LockoutCount,   // lockout_count — número de veces que la cuenta fue bloqueada en el día
		&lockoutDate,      // lockout_date — fecha del primer bloqueo del día (nullable)
		&u.CreatedAt,      // created_at  — timestamp de creación del registro
		&u.UpdatedAt,      // updated_at  — timestamp de última modificación
	)
	if err != nil {
		return nil, err
	}
	u.LockoutDate = lockoutDate
	return &u, nil
}

// userSelectFields lista las columnas de la tabla "users" en el orden que espera scanUser.
// Se usa como literal en los SELECT para mantener consistencia y evitar duplicación.
const userSelectFields = `
	id, username, email, password_hash, is_active, must_change_pwd,
	last_login_at, failed_attempts, locked_until, lockout_count, lockout_date,
	created_at, updated_at `

// FindByUsername busca un usuario por su username en la tabla "users".
// Se usa principalmente en el flujo de login para obtener el usuario antes de
// comparar la contraseña.
// Parámetros:
//   - ctx: contexto de la solicitud HTTP (para cancelación y timeout).
//   - username: nombre de usuario a buscar; la búsqueda es case-sensitive.
//
// Retorna el usuario encontrado, nil si no existe, o un error de base de datos.
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	q := `SELECT` + userSelectFields + `FROM users WHERE username = $1`
	// $1 = username
	row := r.db.QueryRow(ctx, q, username)
	u, err := scanUser(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // usuario no encontrado — no es un error, el caller lo maneja
		}
		return nil, fmt.Errorf("user_repo: find by username: %w", err)
	}
	return u, nil
}

// FindByID busca un usuario por su UUID primario en la tabla "users".
// Se usa cuando ya se conoce el ID del usuario (e.g. desde el JWT o desde otra entidad).
// Parámetros:
//   - ctx: contexto de la solicitud.
//   - id: UUID del usuario a buscar.
//
// Retorna el usuario encontrado, nil si no existe, o un error de base de datos.
func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	q := `SELECT` + userSelectFields + `FROM users WHERE id = $1`
	// $1 = id (UUID)
	row := r.db.QueryRow(ctx, q, id)
	u, err := scanUser(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("user_repo: find by id: %w", err)
	}
	return u, nil
}

// Create inserta un nuevo usuario en la tabla "users".
// Los campos created_at y updated_at se establecen con NOW() en la base de datos.
// El campo password_hash debe recibir ya el hash bcrypt generado por la capa de servicio;
// este repositorio nunca recibe contraseñas en texto claro.
// Retorna un error si username o email ya existen (restricción UNIQUE en la tabla).
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	const q = `
		INSERT INTO users (id, username, email, password_hash, is_active, must_change_pwd, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`
	// $1=id, $2=username, $3=email, $4=password_hash, $5=is_active, $6=must_change_pwd
	_, err := r.db.Exec(ctx, q,
		user.ID, user.Username, user.Email, user.PasswordHash,
		user.IsActive, user.MustChangePwd,
	)
	if err != nil {
		return fmt.Errorf("user_repo: create: %w", err)
	}
	return nil
}

// UpdateFailedAttempts actualiza las columnas de control de bloqueo en la tabla "users"
// después de un intento de login fallido. La lógica de negocio (cuándo bloquear, por
// cuánto tiempo, cuándo hacer el bloqueo permanente) vive en la capa de servicio;
// este método solo persiste los valores ya calculados.
// Parámetros:
//   - userID: UUID del usuario afectado.
//   - attempts: nuevo valor de failed_attempts (contador acumulado desde el último login exitoso).
//   - lockedUntil: timestamp hasta el que la cuenta queda bloqueada; nil si no se bloquea.
//   - lockoutCount: número de veces que la cuenta fue bloqueada hoy.
//   - lockoutDate: fecha del primer bloqueo del día; nil si la cuenta no ha sido bloqueada.
func (r *UserRepository) UpdateFailedAttempts(ctx context.Context, userID uuid.UUID, attempts int, lockedUntil *time.Time, lockoutCount int, lockoutDate *time.Time) error {
	const q = `
		UPDATE users
		SET failed_attempts = $2,
		    locked_until    = $3,
		    lockout_count   = $4,
		    lockout_date    = $5,
		    updated_at      = NOW()
		WHERE id = $1`
	// $1=user_id, $2=failed_attempts, $3=locked_until, $4=lockout_count, $5=lockout_date
	_, err := r.db.Exec(ctx, q, userID, attempts, lockedUntil, lockoutCount, lockoutDate)
	if err != nil {
		return fmt.Errorf("user_repo: update failed attempts: %w", err)
	}
	return nil
}

// UpdateLastLogin registra un login exitoso en la tabla "users".
// Resetea failed_attempts a 0 y limpia locked_until para desbloquear la cuenta
// en caso de que hubiera expirado el bloqueo temporal. También actualiza last_login_at
// con el timestamp actual del servidor de base de datos.
// Parámetros:
//   - userID: UUID del usuario que acaba de autenticarse.
func (r *UserRepository) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error {
	const q = `
		UPDATE users
		SET last_login_at   = NOW(),
		    failed_attempts = 0,
		    locked_until    = NULL,
		    updated_at      = NOW()
		WHERE id = $1`
	// $1 = user_id
	_, err := r.db.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("user_repo: update last login: %w", err)
	}
	return nil
}

// UpdatePassword actualiza la contraseña de un usuario en la tabla "users" y
// limpia la bandera must_change_pwd (la establece en FALSE). Se usa cuando el
// propio usuario cambia su contraseña de manera voluntaria o para cumplir el
// requisito de cambio obligatorio.
// Parámetros:
//   - userID: UUID del usuario cuya contraseña se actualiza.
//   - hash: nuevo hash bcrypt de la contraseña (calculado por la capa de servicio).
func (r *UserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, hash string) error {
	const q = `
		UPDATE users
		SET password_hash  = $2,
		    must_change_pwd = FALSE,
		    updated_at      = NOW()
		WHERE id = $1`
	// $1=user_id, $2=password_hash (nuevo hash bcrypt)
	_, err := r.db.Exec(ctx, q, userID, hash)
	if err != nil {
		return fmt.Errorf("user_repo: update password: %w", err)
	}
	return nil
}

// UpdatePasswordWithFlag actualiza la contraseña de un usuario y permite controlar
// explícitamente el valor de must_change_pwd. Se usa desde el panel de administración
// cuando un admin resetea la contraseña de otro usuario y quiere forzar que ese
// usuario la cambie en su próximo login.
// Parámetros:
//   - userID: UUID del usuario afectado.
//   - hash: nuevo hash bcrypt.
//   - mustChangePwd: TRUE si se requiere que el usuario cambie la contraseña al próximo login.
func (r *UserRepository) UpdatePasswordWithFlag(ctx context.Context, userID uuid.UUID, hash string, mustChangePwd bool) error {
	const q = `
		UPDATE users
		SET password_hash   = $2,
		    must_change_pwd  = $3,
		    updated_at       = NOW()
		WHERE id = $1`
	// $1=user_id, $2=password_hash, $3=must_change_pwd
	_, err := r.db.Exec(ctx, q, userID, hash, mustChangePwd)
	if err != nil {
		return fmt.Errorf("user_repo: update password with flag: %w", err)
	}
	return nil
}

// Unlock desbloquea manualmente una cuenta de usuario, reseteando failed_attempts,
// locked_until y lockout_count. Lo invoca un administrador desde el panel de admin
// cuando necesita desbloquear una cuenta que fue bloqueada permanentemente (lockout_count
// alcanzó el límite diario) o una que aún no expiró.
// Parámetros:
//   - userID: UUID del usuario a desbloquear.
func (r *UserRepository) Unlock(ctx context.Context, userID uuid.UUID) error {
	const q = `
		UPDATE users
		SET failed_attempts = 0,
		    locked_until    = NULL,
		    lockout_count   = 0,
		    updated_at      = NOW()
		WHERE id = $1`
	// $1 = user_id
	_, err := r.db.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("user_repo: unlock: %w", err)
	}
	return nil
}

// UserFilter encapsula los parámetros opcionales para filtrar y paginar la lista de usuarios.
// Todos los campos son opcionales; si no se especifican se devuelven todos los usuarios.
type UserFilter struct {
	Search   string // búsqueda parcial ILIKE (case-insensitive) en username y email
	IsActive *bool  // nil = sin filtro; true = solo activos; false = solo inactivos
	Page     int    // número de página (base 1); se usa para calcular el OFFSET
	PageSize int    // cantidad máxima de resultados por página; máximo 100
}

// List devuelve una lista paginada de usuarios y el total de registros que coinciden
// con el filtro. El total se calcula con una consulta COUNT separada antes de obtener
// los datos, lo que permite al cliente calcular el número de páginas.
// La consulta de datos usa un filtro dinámico: el WHERE se construye en tiempo de
// ejecución agregando condiciones solo para los campos que no son cero/nil, lo que
// evita parámetros innecesarios y mantiene el plan de ejecución eficiente.
// Parámetros:
//   - filter: criterios de búsqueda, filtros y paginación.
//
// Retorna la lista de usuarios, el total de registros (para paginación) y cualquier error.
func (r *UserRepository) List(ctx context.Context, filter UserFilter) ([]*domain.User, int, error) {
	args := []interface{}{}
	where := []string{}
	idx := 1 // índice del siguiente parámetro posicional ($1, $2, …)

	// Filtro de búsqueda: compara username e email con ILIKE para ignorar mayúsculas.
	// Se usa el mismo argumento (like) para ambas columnas, por lo que idx avanza 2.
	if filter.Search != "" {
		where = append(where, fmt.Sprintf("(username ILIKE $%d OR email ILIKE $%d)", idx, idx+1))
		like := "%" + filter.Search + "%"
		args = append(args, like, like) // $idx y $idx+1 apuntan al mismo valor
		idx += 2
	}
	// Filtro por estado activo/inactivo.
	if filter.IsActive != nil {
		where = append(where, fmt.Sprintf("is_active = $%d", idx))
		args = append(args, *filter.IsActive)
		idx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Primera consulta: obtiene el total de registros para calcular total_pages en la API.
	countQ := `SELECT COUNT(*) FROM users ` + whereClause
	var total int
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("user_repo: list count: %w", err)
	}

	// Segunda consulta: obtiene la página de datos solicitada ordenada por fecha de creación descendente.
	// idx y idx+1 corresponden a LIMIT (page_size) y OFFSET calculado a partir de (page-1)*page_size.
	offset := (filter.Page - 1) * filter.PageSize
	dataQ := `SELECT` + userSelectFields + `FROM users ` + whereClause +
		fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("user_repo: list query: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("user_repo: list scan: %w", err)
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// Update modifica los campos mutables de un usuario: username, email e is_active.
// No actualiza la contraseña (ver UpdatePassword) ni los campos de bloqueo (ver
// UpdateFailedAttempts/Unlock). Se usa desde el panel de administración.
// Parámetros:
//   - user: struct con el ID del usuario y los nuevos valores de username, email e is_active.
func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	const q = `
		UPDATE users
		SET username   = $2,
		    email      = $3,
		    is_active  = $4,
		    updated_at = NOW()
		WHERE id = $1`
	// $1=id, $2=username, $3=email, $4=is_active
	_, err := r.db.Exec(ctx, q, user.ID, user.Username, user.Email, user.IsActive)
	if err != nil {
		return fmt.Errorf("user_repo: update: %w", err)
	}
	return nil
}
