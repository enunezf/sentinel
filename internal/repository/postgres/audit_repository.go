// Package postgres implementa los repositorios de persistencia de Sentinel usando
// el driver pgx v5. Ver user_repository.go para una descripción completa del paquete.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enunezf/sentinel/internal/domain"
)

// AuditRepository es el repositorio de la tabla "audit_logs".
// La tabla audit_logs registra un log inmutable de todas las operaciones relevantes
// del sistema (logins, cambios de contraseña, alta de usuarios, asignación de roles,
// etc.). El repositorio solo soporta INSERT y SELECT: no existe UPDATE ni DELETE
// sobre registros de auditoría para garantizar la inmutabilidad del historial.
//
// Consideraciones de tipos PostgreSQL:
//   - ip_address: almacenado como tipo inet de PostgreSQL. En INSERT se pasa como
//     string y PostgreSQL lo convierte automáticamente. En SELECT se castea
//     explícitamente a ip_address::text para que pgx lo escanee como string Go.
//   - old_value / new_value: almacenados como JSONB. Se serializan a JSON antes del
//     INSERT y se deserializan después del SELECT.
//
// Los INSERT se realizan de forma asíncrona desde un canal con buffer de tamaño 1000
// (gestionado por el servicio de auditoría), por lo que este repositorio nunca
// bloquea el hilo de la solicitud HTTP principal.
// Utiliza pgxpool.Pool para el pool de conexiones de pgx.
type AuditRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe
	logger *slog.Logger  // logger estructurado con component="audit_repo"
}

// NewAuditRepository construye un AuditRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewAuditRepository(db *pgxpool.Pool, log *slog.Logger) *AuditRepository {
	return &AuditRepository{
		db:     db,
		logger: log.With("component", "audit_repo"),
	}
}

// Insert escribe una entrada de auditoría en la tabla "audit_logs".
// Los campos old_value y new_value se serializan a JSON antes de insertar para
// almacenarlos en la columna JSONB. Los campos ip_address y error_message son
// opcionales (nullable): se convierten a nil de interfaz cuando están vacíos para
// que PostgreSQL los almacene como NULL en lugar de una cadena vacía.
// created_at se establece con NOW() en la base de datos para consistencia temporal.
// Parámetros:
//   - log: struct domain.AuditLog con todos los datos del evento a registrar.
func (r *AuditRepository) Insert(ctx context.Context, log *domain.AuditLog) error {
	// Serializar old_value a JSON (puede ser nil si no hay valor previo).
	var oldValueJSON, newValueJSON []byte
	var err error
	if log.OldValue != nil {
		oldValueJSON, err = json.Marshal(log.OldValue)
		if err != nil {
			return fmt.Errorf("audit_repo: marshal old_value: %w", err)
		}
	}
	// Serializar new_value a JSON (puede ser nil para eventos de eliminación o login).
	if log.NewValue != nil {
		newValueJSON, err = json.Marshal(log.NewValue)
		if err != nil {
			return fmt.Errorf("audit_repo: marshal new_value: %w", err)
		}
	}

	// ip_address es tipo inet en PostgreSQL. Se pasa como interface{} nil cuando
	// no se dispone de la IP (e.g. eventos internos sin contexto HTTP).
	var ipAddr interface{}
	if log.IPAddress != "" {
		ipAddr = log.IPAddress
	}

	const q = `
		INSERT INTO audit_logs (
			id, event_type, application_id, user_id, actor_id,
			resource_type, resource_id, old_value, new_value,
			ip_address, user_agent, success, error_message, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NOW())`
	// $1 =id            — UUID del evento de auditoría
	// $2 =event_type    — tipo de evento (e.g. "user.login", "password.change")
	// $3 =application_id — app en cuyo contexto ocurrió el evento
	// $4 =user_id       — usuario afectado (puede ser NULL para eventos de sistema)
	// $5 =actor_id      — usuario que realizó la acción (puede coincidir con user_id)
	// $6 =resource_type — tipo de recurso afectado (e.g. "user", "role")
	// $7 =resource_id   — ID del recurso afectado (puede ser NULL)
	// $8 =old_value     — estado anterior del recurso serializado como JSON (JSONB, nullable)
	// $9 =new_value     — estado nuevo del recurso serializado como JSON (JSONB, nullable)
	// $10=ip_address    — IP del cliente como texto; PostgreSQL la convierte a tipo inet
	// $11=user_agent    — User-Agent HTTP del cliente
	// $12=success       — TRUE si la operación fue exitosa; FALSE si falló
	// $13=error_message — mensaje de error en caso de fallo (nullable)
	_, err = r.db.Exec(ctx, q,
		log.ID, string(log.EventType), log.ApplicationID, log.UserID, log.ActorID,
		log.ResourceType, log.ResourceID,
		nullableJSON(oldValueJSON), nullableJSON(newValueJSON),
		ipAddr, log.UserAgent, log.Success, nullableString(log.ErrorMessage),
	)
	if err != nil {
		return fmt.Errorf("audit_repo: insert: %w", err)
	}
	return nil
}

// nullableJSON convierte un slice de bytes JSON a interface{}: devuelve nil si
// el slice está vacío (campo no aplica) o el JSON como string para su almacenamiento
// en la columna JSONB de PostgreSQL.
func nullableJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

// nullableString convierte una cadena vacía a nil de interfaz para almacenarla
// como NULL en PostgreSQL. Se usa para campos opcionales como error_message.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// AuditFilter encapsula los parámetros opcionales para filtrar y paginar el historial
// de auditoría. Todos los campos son opcionales; si ninguno está establecido se
// devuelven todos los registros (paginados).
type AuditFilter struct {
	UserID        *uuid.UUID // filtra eventos donde user_id = UserID (usuario afectado)
	ActorID       *uuid.UUID // filtra eventos donde actor_id = ActorID (usuario que actuó)
	EventType     string     // filtra por tipo de evento exacto (e.g. "user.login")
	FromDate      *time.Time // filtra eventos creados a partir de esta fecha (inclusive)
	ToDate        *time.Time // filtra eventos creados hasta esta fecha (inclusive)
	ApplicationID *uuid.UUID // filtra eventos de una aplicación específica
	Success       *bool      // nil = todos; true = solo exitosos; false = solo fallidos
	Page          int        // número de página (base 1)
	PageSize      int        // registros por página; máximo 100
}

// List devuelve una lista paginada de entradas de auditoría que coinciden con el filtro
// y el total de registros para calcular el número de páginas.
// El WHERE se construye dinámicamente: se acumulan condiciones AND solo para los
// filtros que tienen valor, lo que permite combinaciones arbitrarias de filtros sin
// parámetros vacíos.
//
// Nota sobre ip_address::text: la columna ip_address es tipo inet en PostgreSQL, que
// no mapea directamente a string en Go con pgx. Se castea explícitamente a text en
// el SELECT para que pgx pueda escanearlo en una variable *string de Go. En el INSERT
// se hace el proceso inverso: PostgreSQL convierte el string al tipo inet.
//
// Parámetros:
//   - filter: criterios de filtrado y paginación.
//
// Retorna la lista de entradas de auditoría (nunca nil), el total de registros y cualquier error.
func (r *AuditRepository) List(ctx context.Context, filter AuditFilter) ([]*domain.AuditLog, int, error) {
	args := []interface{}{}
	where := []string{}
	idx := 1 // índice del próximo parámetro posicional

	// Construir filtros dinámicamente: solo se agrega la condición si el campo tiene valor.
	if filter.UserID != nil {
		where = append(where, fmt.Sprintf("user_id = $%d", idx))
		args = append(args, *filter.UserID)
		idx++
	}
	if filter.ActorID != nil {
		where = append(where, fmt.Sprintf("actor_id = $%d", idx))
		args = append(args, *filter.ActorID)
		idx++
	}
	if filter.EventType != "" {
		where = append(where, fmt.Sprintf("event_type = $%d", idx))
		args = append(args, filter.EventType)
		idx++
	}
	// Rango de fechas: FromDate filtra desde (inclusive), ToDate hasta (inclusive).
	if filter.FromDate != nil {
		where = append(where, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, *filter.FromDate)
		idx++
	}
	if filter.ToDate != nil {
		where = append(where, fmt.Sprintf("created_at <= $%d", idx))
		args = append(args, *filter.ToDate)
		idx++
	}
	if filter.ApplicationID != nil {
		where = append(where, fmt.Sprintf("application_id = $%d", idx))
		args = append(args, *filter.ApplicationID)
		idx++
	}
	if filter.Success != nil {
		where = append(where, fmt.Sprintf("success = $%d", idx))
		args = append(args, *filter.Success)
		idx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Primera consulta: total de registros para calcular total_pages.
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs `+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit_repo: list count: %w", err)
	}

	// Segunda consulta: datos paginados.
	// ip_address::text: cast necesario porque el tipo inet de PostgreSQL no mapea
	// directamente a string en Go; sin el cast, pgx devolvería un error de tipo.
	offset := (filter.Page - 1) * filter.PageSize
	dataQ := `
		SELECT id, event_type, application_id, user_id, actor_id,
		       resource_type, resource_id, old_value, new_value,
		       ip_address::text, user_agent, success, error_message, created_at
		FROM audit_logs ` + whereClause +
		fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, idx, idx+1)
	// idx   = LIMIT  (page_size)
	// idx+1 = OFFSET (page-1) * page_size
	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit_repo: list query: %w", err)
	}
	defer rows.Close()

	// make inicializa un slice vacío (no nil) para que el marshaling a JSON
	// devuelva [] en lugar de null cuando no hay resultados.
	logs := make([]*domain.AuditLog, 0)
	for rows.Next() {
		var l domain.AuditLog
		var oldValRaw, newValRaw []byte // JSON raw de old_value y new_value (JSONB)
		var ipAddr *string              // nullable: se asigna a l.IPAddress solo si no es nil
		var errorMsg *string            // nullable: se asigna a l.ErrorMessage solo si no es nil
		err := rows.Scan(
			&l.ID, &l.EventType, &l.ApplicationID, &l.UserID, &l.ActorID,
			&l.ResourceType, &l.ResourceID, &oldValRaw, &newValRaw,
			&ipAddr, &l.UserAgent, &l.Success, &errorMsg, &l.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("audit_repo: scan: %w", err)
		}
		// Desempaquetar punteros nullable al struct de dominio.
		if ipAddr != nil {
			l.IPAddress = *ipAddr
		}
		if errorMsg != nil {
			l.ErrorMessage = *errorMsg
		}
		// Deserializar JSONB a map[string]interface{} del dominio (puede ser nil si la columna era NULL).
		if len(oldValRaw) > 0 {
			_ = json.Unmarshal(oldValRaw, &l.OldValue)
		}
		if len(newValRaw) > 0 {
			_ = json.Unmarshal(newValRaw, &l.NewValue)
		}
		logs = append(logs, &l)
	}
	return logs, total, rows.Err()
}
