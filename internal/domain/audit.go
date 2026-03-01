// Package domain contiene los tipos de datos centrales del sistema Sentinel.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// EventType identifica el tipo de evento registrado en el log de auditoría.
// Cada constante representa una acción de seguridad específica que ocurrió en el sistema.
// El formato es DOMINIO_ACCION en mayúsculas con guiones bajos (screaming snake case).
type EventType string

// Eventos de autenticación. Se generan durante el ciclo de vida de la sesión del usuario.
const (
	// EventAuthLoginSuccess se registra cuando un usuario inicia sesión exitosamente.
	EventAuthLoginSuccess EventType = "AUTH_LOGIN_SUCCESS"

	// EventAuthLoginFailed se registra cuando un intento de inicio de sesión falla
	// por credenciales incorrectas o cuenta bloqueada.
	EventAuthLoginFailed EventType = "AUTH_LOGIN_FAILED"

	// EventAuthLogout se registra cuando el usuario cierra sesión explícitamente,
	// lo que invalida el refresh token en Redis y PostgreSQL.
	EventAuthLogout EventType = "AUTH_LOGOUT"

	// EventAuthTokenRefreshed se registra cuando se usa un refresh token válido
	// para generar un nuevo access token JWT.
	EventAuthTokenRefreshed EventType = "AUTH_TOKEN_REFRESHED"

	// EventAuthPasswordChanged se registra cuando el propio usuario cambia su contraseña
	// mediante el endpoint /auth/change-password.
	EventAuthPasswordChanged EventType = "AUTH_PASSWORD_CHANGED"

	// EventAuthPasswordReset se registra cuando un administrador restablece la contraseña
	// de otro usuario mediante el endpoint /admin/users/:id/reset-password.
	EventAuthPasswordReset EventType = "AUTH_PASSWORD_RESET"

	// EventAuthAccountLocked se registra cuando la cuenta de un usuario queda bloqueada
	// automáticamente al superar el número máximo de intentos fallidos configurado.
	EventAuthAccountLocked EventType = "AUTH_ACCOUNT_LOCKED"
)

// Eventos de autorización. Se generan al evaluar si un usuario tiene un permiso.
const (
	// EventAuthzPermissionGranted se registra cuando la verificación de permiso
	// resulta exitosa (el usuario posee el permiso solicitado).
	EventAuthzPermissionGranted EventType = "AUTHZ_PERMISSION_GRANTED"

	// EventAuthzPermissionDenied se registra cuando la verificación de permiso
	// falla (el usuario NO posee el permiso solicitado).
	EventAuthzPermissionDenied EventType = "AUTHZ_PERMISSION_DENIED"
)

// Eventos de gestión de usuarios. Se generan desde el panel de administración.
const (
	// EventUserCreated se registra cuando un administrador crea una nueva cuenta de usuario.
	EventUserCreated EventType = "USER_CREATED"

	// EventUserUpdated se registra cuando se modifican los datos de un usuario existente,
	// como su email o estado de activación.
	EventUserUpdated EventType = "USER_UPDATED"

	// EventUserDeactivated se registra cuando un administrador desactiva una cuenta de usuario.
	// La desactivación impide nuevos inicios de sesión sin eliminar el historial.
	EventUserDeactivated EventType = "USER_DEACTIVATED"

	// EventUserUnlocked se registra cuando un administrador desbloquea manualmente
	// una cuenta que fue bloqueada por exceso de intentos fallidos.
	EventUserUnlocked EventType = "USER_UNLOCKED"
)

// Eventos de gestión de roles. Se generan desde el panel de administración.
const (
	// EventRoleCreated se registra cuando se crea un rol nuevo en una aplicación.
	EventRoleCreated EventType = "ROLE_CREATED"

	// EventRoleUpdated se registra cuando se modifican los datos de un rol existente.
	EventRoleUpdated EventType = "ROLE_UPDATED"

	// EventRoleDeleted se registra cuando se elimina un rol (solo roles no del sistema).
	EventRoleDeleted EventType = "ROLE_DELETED"

	// EventRolePermissionAssigned se registra cuando se agrega un permiso a un rol.
	EventRolePermissionAssigned EventType = "ROLE_PERMISSION_ASSIGNED"

	// EventRolePermissionRevoked se registra cuando se quita un permiso de un rol.
	EventRolePermissionRevoked EventType = "ROLE_PERMISSION_REVOKED"
)

// Eventos de asignaciones. Se generan cuando se vinculan entidades a usuarios.
const (
	// EventUserRoleAssigned se registra cuando se asigna un rol a un usuario.
	EventUserRoleAssigned EventType = "USER_ROLE_ASSIGNED"

	// EventUserRoleRevoked se registra cuando se revoca un rol previamente asignado.
	EventUserRoleRevoked EventType = "USER_ROLE_REVOKED"

	// EventUserPermissionAssigned se registra cuando se otorga un permiso directo a un usuario,
	// sin pasar por un rol.
	EventUserPermissionAssigned EventType = "USER_PERMISSION_ASSIGNED"

	// EventUserPermissionRevoked se registra cuando se revoca un permiso directo de un usuario.
	EventUserPermissionRevoked EventType = "USER_PERMISSION_REVOKED"

	// EventUserCostCenterAssigned se registra cuando se asigna uno o más centros de costo
	// a un usuario para una aplicación específica.
	EventUserCostCenterAssigned EventType = "USER_COST_CENTER_ASSIGNED"
)

// Eventos del sistema. Se generan durante operaciones de infraestructura del servicio.
const (
	// EventSystemBootstrap se registra una única vez cuando el sistema se inicializa
	// por primera vez: crea la aplicación raíz, el usuario administrador y los roles del sistema.
	EventSystemBootstrap EventType = "SYSTEM_BOOTSTRAP"
)

// AuditLog representa un registro de auditoría inmutable en el sistema Sentinel.
// Cada evento de seguridad relevante (login, cambio de contraseña, asignación de roles, etc.)
// genera un AuditLog que se persiste de forma asíncrona en la base de datos.
// Una vez creado, ningún campo debe modificarse; los registros son de solo escritura.
//
// El servicio de auditoría escribe estos registros a través de un canal con buffer (tamaño 1000)
// para no bloquear la respuesta HTTP principal. Un fallo al escribir el log no debe
// interrumpir la operación de negocio que lo originó.
type AuditLog struct {
	// ID es el identificador único del registro de auditoría (UUID v4).
	// Se genera en el momento de la inserción.
	ID uuid.UUID `json:"id"`

	// EventType identifica qué tipo de evento ocurrió.
	// Permite filtrar y analizar eventos específicos en el panel de auditoría.
	EventType EventType `json:"event_type"`

	// ApplicationID referencia la aplicación en cuyo contexto ocurrió el evento.
	// Es nil en eventos del sistema que no están asociados a una aplicación específica.
	ApplicationID *uuid.UUID `json:"application_id"`

	// UserID es el UUID del usuario sobre el cual se realizó la acción.
	// Por ejemplo, en un EVENT_USER_UPDATED, es el usuario que fue modificado.
	// Es nil cuando la acción no involucra a un usuario específico como objeto.
	UserID *uuid.UUID `json:"user_id"`

	// ActorID es el UUID del usuario o proceso que realizó la acción.
	// Puede diferir de UserID cuando un administrador actúa sobre otro usuario.
	// Es nil en eventos iniciados por procesos internos (como el bootstrap).
	ActorID *uuid.UUID `json:"actor_id"`

	// ResourceType describe el tipo de entidad sobre la que se realizó la acción.
	// Ejemplos: "user", "role", "permission", "cost_center".
	// Es nil cuando el evento no está asociado a un recurso específico.
	ResourceType *string `json:"resource_type"`

	// ResourceID es el UUID de la entidad específica sobre la que se realizó la acción.
	// Es nil cuando el evento no está asociado a un recurso identificable por ID.
	ResourceID *uuid.UUID `json:"resource_id"`

	// OldValue almacena el estado anterior de la entidad modificada, en formato JSON.
	// Se usa en eventos de actualización para registrar qué cambió exactamente.
	// Es nil para eventos de creación o cuando no hay estado previo relevante.
	OldValue map[string]interface{} `json:"old_value"`

	// NewValue almacena el nuevo estado de la entidad modificada, en formato JSON.
	// Se usa en eventos de actualización o creación para registrar el resultado.
	// Nunca debe contener datos sensibles como contraseñas o tokens.
	NewValue map[string]interface{} `json:"new_value"`

	// IPAddress es la dirección IP del cliente que originó la solicitud HTTP.
	// Se obtiene del header X-Forwarded-For o de la conexión directa.
	IPAddress string `json:"ip_address"`

	// UserAgent es el valor del header User-Agent de la solicitud HTTP.
	// Permite identificar el tipo de cliente (navegador, app móvil, etc.).
	UserAgent string `json:"user_agent"`

	// Success indica si la operación auditada se completó exitosamente.
	// Un valor false con ErrorMessage no vacío describe una operación fallida.
	Success bool `json:"success"`

	// ErrorMessage describe el motivo del fallo cuando Success es false.
	// Vacío en registros de operaciones exitosas.
	ErrorMessage string `json:"error_message"`

	// CreatedAt es la fecha y hora de creación del registro de auditoría.
	// No tiene campo UpdatedAt porque los registros son inmutables.
	CreatedAt time.Time `json:"created_at"`
}
