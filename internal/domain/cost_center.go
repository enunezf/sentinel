// Package domain contiene los tipos de datos centrales del sistema Sentinel.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// CostCenter representa un centro de costo (unidad organizacional o contable)
// dentro de una aplicación registrada en Sentinel.
// Los centros de costo permiten agrupar usuarios según su área funcional o
// departamento. Esta información se incluye en el mapa de permisos del usuario
// para que las aplicaciones consumidoras puedan aplicar lógica de negocio
// basada en la unidad organizacional del actor autenticado.
type CostCenter struct {
	// ID es el identificador único del centro de costo (UUID v4).
	ID uuid.UUID `json:"id"`

	// ApplicationID referencia la aplicación a la que pertenece el centro de costo.
	// Cada aplicación gestiona sus propios centros de costo de forma independiente.
	ApplicationID uuid.UUID `json:"application_id"`

	// Code es el código corto único del centro de costo dentro de la aplicación.
	// Suele corresponder al código contable o al código de área de la organización.
	// Ejemplos: "CC-001", "VENTAS", "TI-SUB".
	Code string `json:"code"`

	// Name es el nombre descriptivo del centro de costo, legible por humanos.
	// Ejemplos: "Gerencia de Ventas", "Departamento de TI".
	Name string `json:"name"`

	// IsActive indica si el centro de costo está habilitado.
	// Los centros de costo inactivos no aparecen en las consultas normales
	// y no se incluyen en el mapa de permisos del usuario.
	IsActive bool `json:"is_active"`

	// CreatedAt es la fecha y hora de creación del registro.
	CreatedAt time.Time `json:"created_at"`
}

// UserCostCenter representa la asignación de un centro de costo a un usuario
// para una aplicación específica. Un usuario puede pertenecer a múltiples
// centros de costo simultáneamente (por ejemplo, un gerente que cubre dos áreas).
// La asignación admite vigencia temporal mediante ValidFrom y ValidUntil.
type UserCostCenter struct {
	// UserID referencia el usuario al que se le asigna el centro de costo.
	UserID uuid.UUID

	// CostCenterID referencia el centro de costo que se asigna.
	CostCenterID uuid.UUID

	// ApplicationID referencia la aplicación en cuyo contexto es válida la asignación.
	ApplicationID uuid.UUID

	// GrantedBy es el UUID del administrador que realizó la asignación.
	// Se registra para trazabilidad y auditoría.
	GrantedBy uuid.UUID

	// ValidFrom es la fecha a partir de la cual la asignación está activa.
	ValidFrom time.Time

	// ValidUntil es la fecha opcional de expiración de la asignación.
	// Si es nil, la asignación no tiene fecha de vencimiento.
	ValidUntil *time.Time

	// CostCenterCode es el código del centro de costo, populado mediante JOIN.
	// No se almacena en user_cost_centers; se carga al consultar asignaciones del usuario.
	// Populated via join.
	CostCenterCode string

	// CostCenterName es el nombre del centro de costo, populado mediante JOIN.
	// Se incluye para mostrar información legible sin consultas adicionales.
	// Populated via join.
	CostCenterName string
}
