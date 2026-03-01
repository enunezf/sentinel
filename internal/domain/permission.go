// Package domain contiene los tipos de datos centrales del sistema Sentinel.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// ScopeType clasifica el alcance de un permiso dentro del sistema RBAC.
// Permite estructurar los permisos en jerarquías para facilitar su gestión
// y definir con precisión qué operaciones están autorizadas.
type ScopeType string

const (
	// ScopeTypeGlobal indica que el permiso aplica a todos los recursos del sistema,
	// sin restricción de módulo ni de entidad. Usado para permisos de superadministrador.
	ScopeTypeGlobal ScopeType = "global"

	// ScopeTypeModule indica que el permiso aplica a un módulo funcional completo,
	// como "facturación" o "reportes", sin importar el recurso específico dentro de él.
	ScopeTypeModule ScopeType = "module"

	// ScopeTypeResource indica que el permiso aplica a un tipo de entidad específico,
	// por ejemplo "usuario" o "pedido", independientemente de la acción a realizar.
	ScopeTypeResource ScopeType = "resource"

	// ScopeTypeAction indica que el permiso aplica a una acción concreta sobre un recurso,
	// por ejemplo "users.create" o "orders.delete". Es el nivel más granular.
	ScopeTypeAction ScopeType = "action"
)

// IsValidScopeType informa si el valor st es uno de los ScopeType admitidos por el sistema.
// Se usa para validar la entrada del usuario antes de persistir un permiso nuevo.
//
// Parámetros:
//   - st: cadena a validar (por ejemplo "global", "module", "resource", "action").
//
// Retorna true si st corresponde a un ScopeType válido, false en caso contrario.
func IsValidScopeType(st string) bool {
	switch ScopeType(st) {
	case ScopeTypeGlobal, ScopeTypeModule, ScopeTypeResource, ScopeTypeAction:
		return true
	}
	return false
}

// Permission representa un permiso de autorización para una aplicación registrada.
// Los permisos son códigos únicos por aplicación (por ejemplo "admin.users.read")
// que se asignan a roles o directamente a usuarios para controlar el acceso a recursos.
// Los backends consumidores verifican estos permisos llamando al endpoint /authz/verify.
type Permission struct {
	// ID es el identificador único del permiso (UUID v4).
	ID uuid.UUID `json:"id"`

	// ApplicationID referencia la aplicación a la que pertenece este permiso.
	// Un mismo código puede existir en distintas aplicaciones sin conflicto.
	ApplicationID uuid.UUID `json:"application_id"`

	// Code es el código único del permiso dentro de la aplicación.
	// Se recomienda usar notación punteada jerárquica: "modulo.recurso.accion".
	// Ejemplos: "admin.users.read", "orders.create", "reports.export".
	Code string `json:"code"`

	// Description explica en lenguaje natural para qué sirve el permiso.
	// Visible en el panel de administración para facilitar la asignación de roles.
	Description string `json:"description"`

	// ScopeType clasifica el alcance del permiso (global, module, resource, action).
	// Permite filtrar y organizar permisos por nivel de granularidad.
	ScopeType ScopeType `json:"scope_type"`

	// CreatedAt es la fecha y hora de creación del registro.
	CreatedAt time.Time `json:"created_at"`
}

// UserPermission representa la asignación directa de un permiso a un usuario,
// sin pasar por un rol. Esto permite otorgar accesos puntuales y excepcionales
// que no justifican la creación de un rol específico.
// Al igual que UserRole, admite vigencia temporal mediante ValidFrom y ValidUntil.
type UserPermission struct {
	// ID es el identificador único de la asignación directa (UUID v4).
	ID uuid.UUID

	// UserID referencia el usuario que recibe el permiso.
	UserID uuid.UUID

	// PermissionID referencia el permiso que se otorga directamente.
	PermissionID uuid.UUID

	// ApplicationID referencia la aplicación en cuyo contexto es válida la asignación.
	ApplicationID uuid.UUID

	// GrantedBy es el UUID del administrador que realizó la asignación directa.
	// Se registra para trazabilidad y auditoría.
	GrantedBy uuid.UUID

	// ValidFrom es la fecha a partir de la cual el permiso directo está activo.
	ValidFrom time.Time

	// ValidUntil es la fecha opcional de expiración del permiso directo.
	// Si es nil, el permiso no tiene fecha de vencimiento.
	ValidUntil *time.Time

	// IsActive indica si la asignación directa está vigente.
	// Se establece en false al revocar el permiso (revocación lógica).
	IsActive bool

	// CreatedAt es la fecha y hora de creación del registro de asignación.
	CreatedAt time.Time

	// PermissionCode es el código del permiso, populado mediante JOIN con la tabla permissions.
	// No se almacena en user_permissions; se carga al consultar asignaciones del usuario.
	// Populated via join.
	PermissionCode string
}
