// Package domain contiene los tipos de datos centrales del sistema Sentinel.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Role representa un rol de seguridad dentro de una aplicación registrada en Sentinel.
// Un rol es una agrupación con nombre que contiene un conjunto de permisos.
// Los usuarios reciben roles a través de la asignación UserRole, y heredan
// todos los permisos asociados a cada rol asignado.
// Los roles son por aplicación: el mismo nombre puede existir en distintas aplicaciones.
type Role struct {
	// ID es el identificador único del rol (UUID v4).
	ID uuid.UUID

	// ApplicationID referencia la aplicación a la que pertenece este rol.
	// Un rol solo es válido dentro del contexto de su aplicación.
	ApplicationID uuid.UUID

	// Name es el nombre del rol, único dentro de la misma aplicación.
	// Ejemplos: "administrador", "operador", "auditor".
	Name string

	// Description es una explicación opcional del propósito del rol,
	// visible en el panel de administración para facilitar su gestión.
	Description string

	// IsSystem indica si el rol fue creado durante el bootstrap del sistema
	// y no puede ser eliminado manualmente. Por ejemplo, el rol "admin" raíz.
	IsSystem bool

	// IsActive indica si el rol está habilitado. Un rol inactivo no otorga
	// permisos a los usuarios que lo tengan asignado.
	IsActive bool

	// CreatedAt es la fecha y hora de creación del registro.
	CreatedAt time.Time

	// UpdatedAt es la fecha y hora de la última modificación del registro.
	UpdatedAt time.Time
}

// UserRole representa la asignación de un rol a un usuario para una aplicación específica.
// Una misma asignación puede tener vigencia temporal (ValidFrom / ValidUntil),
// lo que permite otorgar accesos por períodos definidos sin necesidad de revocarlos manualmente.
type UserRole struct {
	// ID es el identificador único de la asignación (UUID v4).
	ID uuid.UUID

	// UserID referencia el usuario al que se le asigna el rol.
	UserID uuid.UUID

	// RoleID referencia el rol que se asigna.
	RoleID uuid.UUID

	// ApplicationID referencia la aplicación en cuyo contexto es válida la asignación.
	// Debe coincidir con el ApplicationID del Role.
	ApplicationID uuid.UUID

	// GrantedBy es el UUID del administrador que realizó la asignación.
	// Se registra para trazabilidad y auditoría.
	GrantedBy uuid.UUID

	// ValidFrom es la fecha a partir de la cual la asignación está activa.
	// Normalmente es la fecha de creación, pero puede ser una fecha futura.
	ValidFrom time.Time

	// ValidUntil es la fecha opcional de expiración de la asignación.
	// Si es nil, la asignación no tiene fecha de vencimiento.
	ValidUntil *time.Time

	// IsActive indica si la asignación está vigente. Un administrador puede
	// revocar un rol estableciendo este campo en false (revocación lógica).
	IsActive bool

	// CreatedAt es la fecha y hora de creación del registro de asignación.
	CreatedAt time.Time

	// RoleName es el nombre del rol, populado mediante JOIN con la tabla roles.
	// No se almacena directamente en la tabla user_roles; se carga al consultar.
	// Populated via join.
	RoleName string
}

// RoleWithPermissions extiende Role incluyendo la lista de permisos asociados
// y el número de usuarios que tienen asignado el rol.
// Se utiliza en los endpoints de consulta de detalle de un rol desde el panel de administración.
type RoleWithPermissions struct {
	// Role embebe todos los campos del rol base.
	Role

	// Permissions es la lista de permisos asignados a este rol.
	// Se obtiene mediante un JOIN con las tablas role_permissions y permissions.
	Permissions []Permission

	// UsersCount es la cantidad de usuarios activos que tienen este rol asignado.
	// Se calcula con COUNT en la consulta y sirve para mostrar el impacto de un cambio.
	UsersCount int
}
