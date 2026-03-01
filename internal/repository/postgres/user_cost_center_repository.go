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

// UserCostCenterRepository es el repositorio de la tabla "user_cost_centers".
// La tabla user_cost_centers asigna centros de costo a usuarios dentro de una
// aplicación. Esta asignación se usa en la autorización de alcance restringido:
// cuando un permiso tiene scope_type="cost_center", el sistema verifica que el
// usuario tenga acceso al cost center sobre el que quiere operar.
// La tabla usa clave compuesta (user_id, cost_center_id) con restricción UNIQUE,
// por lo que Assign usa ON CONFLICT DO UPDATE (upsert) para actualizar la vigencia
// si la asignación ya existe.
// Utiliza pgxpool.Pool para el pool de conexiones de pgx.
type UserCostCenterRepository struct {
	db     *pgxpool.Pool // pool de conexiones a PostgreSQL; thread-safe
	logger *slog.Logger  // logger estructurado con component="user_cc_repo"
}

// NewUserCostCenterRepository construye un UserCostCenterRepository listo para usar.
// Recibe db, el pool de conexiones a PostgreSQL, y log, el logger raíz de la
// aplicación. Devuelve un puntero al repositorio inicializado.
func NewUserCostCenterRepository(db *pgxpool.Pool, log *slog.Logger) *UserCostCenterRepository {
	return &UserCostCenterRepository{
		db:     db,
		logger: log.With("component", "user_cc_repo"),
	}
}

// Assign crea o actualiza la asignación de un cost center a un usuario.
// La cláusula ON CONFLICT DO UPDATE implementa un upsert: si ya existe la
// asignación (mismo user_id y cost_center_id), en lugar de fallar actualiza
// valid_from, valid_until y granted_by con los nuevos valores. Esto permite
// renovar la vigencia de una asignación existente con una sola operación.
// Parámetros:
//   - ucc: struct domain.UserCostCenter con todos los campos necesarios.
func (r *UserCostCenterRepository) Assign(ctx context.Context, ucc *domain.UserCostCenter) error {
	const q = `
		INSERT INTO user_cost_centers (user_id, cost_center_id, application_id, granted_by, valid_from, valid_until)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, cost_center_id) DO UPDATE
		SET valid_from = EXCLUDED.valid_from, valid_until = EXCLUDED.valid_until, granted_by = EXCLUDED.granted_by`
	// $1=user_id, $2=cost_center_id, $3=application_id
	// $4=granted_by (UUID del admin que asignó el cost center)
	// $5=valid_from, $6=valid_until (nullable)
	// ON CONFLICT: si (user_id, cost_center_id) ya existe, actualiza la vigencia
	_, err := r.db.Exec(ctx, q,
		ucc.UserID, ucc.CostCenterID, ucc.ApplicationID,
		ucc.GrantedBy, ucc.ValidFrom, ucc.ValidUntil,
	)
	if err != nil {
		return fmt.Errorf("user_cc_repo: assign: %w", err)
	}
	return nil
}

// ListForUser devuelve todas las asignaciones de cost centers de un usuario en
// todas las aplicaciones. Realiza un JOIN con "cost_centers" para incluir el
// código y nombre de cada cost center, evitando un SELECT adicional por fila.
// Ordena por cc.code ASC para una presentación consistente.
// Parámetros:
//   - userID: UUID del usuario cuyas asignaciones se quieren listar.
//
// Retorna la lista de asignaciones (puede estar vacía) o un error de base de datos.
func (r *UserCostCenterRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]*domain.UserCostCenter, error) {
	const q = `
		SELECT ucc.user_id, ucc.cost_center_id, ucc.application_id, ucc.granted_by,
		       ucc.valid_from, ucc.valid_until, cc.code, cc.name
		FROM user_cost_centers ucc
		JOIN cost_centers cc ON cc.id = ucc.cost_center_id
		WHERE ucc.user_id = $1
		ORDER BY cc.code ASC`
	// $1 = user_id
	// JOIN: user_cost_centers ucc → cost_centers cc (para obtener cc.code y cc.name)
	rows, err := r.db.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("user_cc_repo: list for user: %w", err)
	}
	defer rows.Close()

	var result []*domain.UserCostCenter
	for rows.Next() {
		var ucc domain.UserCostCenter
		if err := rows.Scan(&ucc.UserID, &ucc.CostCenterID, &ucc.ApplicationID, &ucc.GrantedBy,
			&ucc.ValidFrom, &ucc.ValidUntil, &ucc.CostCenterCode, &ucc.CostCenterName); err != nil {
			return nil, fmt.Errorf("user_cc_repo: scan: %w", err)
		}
		result = append(result, &ucc)
	}
	return result, rows.Err()
}

// GetActiveCodesForUserApp devuelve los códigos de los cost centers activos de un
// usuario en una aplicación específica. Se usa en el servicio de autorización para
// verificar si un usuario puede operar sobre un cost center concreto cuando el
// permiso tiene scope_type="cost_center".
//
// Una asignación se considera activa si cumple todas estas condiciones:
//   - ucc.valid_from <= NOW() (la asignación ya entró en vigencia)
//   - ucc.valid_until IS NULL OR ucc.valid_until > NOW() (no ha expirado)
//
// Nota: a diferencia de user_roles y user_permissions, user_cost_centers no tiene
// columna is_active; la vigencia se controla únicamente mediante las fechas.
//
// Parámetros:
//   - userID: UUID del usuario.
//   - appID:  UUID de la aplicación.
//
// Retorna los códigos de cost centers activos (e.g. ["CC-001", "CC-002"]) o un error.
func (r *UserCostCenterRepository) GetActiveCodesForUserApp(ctx context.Context, userID, appID uuid.UUID) ([]string, error) {
	const q = `
		SELECT cc.code
		FROM user_cost_centers ucc
		JOIN cost_centers cc ON cc.id = ucc.cost_center_id
		WHERE ucc.user_id = $1
		  AND ucc.application_id = $2
		  AND ucc.valid_from <= NOW()
		  AND (ucc.valid_until IS NULL OR ucc.valid_until > NOW())`
	// $1=user_id, $2=application_id
	// JOIN: user_cost_centers ucc → cost_centers cc (para obtener el código legible)
	rows, err := r.db.Query(ctx, q, userID, appID)
	if err != nil {
		return nil, fmt.Errorf("user_cc_repo: get active codes: %w", err)
	}
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("user_cc_repo: scan code: %w", err)
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}
