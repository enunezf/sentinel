// Package handler (swagger_types.go) define los tipos de datos usados
// exclusivamente como esquemas en las anotaciones de Swagger/OpenAPI.
//
// IMPORTANTE: Ninguno de estos tipos se usa en la lógica de negocio en tiempo
// de ejecución. Los handlers retornan fiber.Map{} directamente, lo que es más
// eficiente. Estos structs existen únicamente para que la herramienta "swag"
// pueda generar los JSON schemas correctos en la documentación OpenAPI.
//
// Cuando se agregue o modifique un endpoint, actualizar el tipo correspondiente
// aquí y regenerar la documentación con:
//
//	~/go/bin/swag init --generalInfo cmd/server/main.go --output docs/api \
//	  --parseDependency --parseInternal --exclude web/
package handler

// Este archivo define structs utilizados exclusivamente para las anotaciones de Swagger.
// Los handlers internamente usan fiber.Map{}; estos tipos solo sirven para que swag
// genere los JSON schemas correctamente en la documentación.

// ─── REQUESTS ──────────────────────────────────────────────────────────────────

// SwaggerLoginRequest es el esquema del cuerpo de POST /auth/login.
// client_type determina el TTL del refresh token (web=7d, mobile/desktop=30d).
type SwaggerLoginRequest struct {
	Username   string `json:"username" example:"admin"`
	Password   string `json:"password" example:"Admin@Local1!"`
	ClientType string `json:"client_type" enums:"web,mobile,desktop" example:"web"` // enum: web | mobile | desktop
}

// SwaggerRefreshRequest es el esquema del cuerpo de POST /auth/refresh.
// refresh_token es el UUID v4 raw entregado por el login.
type SwaggerRefreshRequest struct {
	RefreshToken string `json:"refresh_token" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// SwaggerChangePasswordRequest es el esquema del cuerpo de POST /auth/change-password.
type SwaggerChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" example:"Admin@Local1!"`  // contraseña actual para verificar identidad
	NewPassword     string `json:"new_password" example:"NuevaClave@2025!"`   // nueva contraseña que debe cumplir la política
}

// SwaggerVerifyPermissionRequest es el esquema del cuerpo de POST /authz/verify.
// cost_center_id es opcional; si se omite la verificación es global.
type SwaggerVerifyPermissionRequest struct {
	Permission   string `json:"permission" example:"admin.users.read"`
	CostCenterID string `json:"cost_center_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"` // opcional
}

// SwaggerCreateUserRequest es el esquema del cuerpo de POST /admin/users.
// La contraseña debe cumplir la política de seguridad del sistema.
type SwaggerCreateUserRequest struct {
	Username string `json:"username" example:"jdoe"`
	Email    string `json:"email" example:"jdoe@empresa.com"`
	Password string `json:"password" example:"Clave@Segura1!"` // debe cumplir política de contraseñas
}

// SwaggerUpdateUserRequest es el esquema del cuerpo de PUT /admin/users/:id.
// Todos los campos son opcionales; los omitidos conservan su valor actual.
type SwaggerUpdateUserRequest struct {
	Username *string `json:"username,omitempty" example:"jdoe_nuevo"` // opcional, nuevo username
	Email    *string `json:"email,omitempty" example:"nuevo@empresa.com"` // opcional, nuevo email
	IsActive *bool   `json:"is_active,omitempty" example:"true"`      // opcional, activar/desactivar cuenta
}

// SwaggerAssignRoleRequest es el esquema del cuerpo de POST /admin/users/:id/roles.
// valid_from y valid_until son opcionales; si se omiten la asignación es permanente.
type SwaggerAssignRoleRequest struct {
	RoleID     string  `json:"role_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	ValidFrom  *string `json:"valid_from,omitempty" example:"2025-01-01T00:00:00Z"`  // inicio de vigencia (RFC3339)
	ValidUntil *string `json:"valid_until,omitempty" example:"2025-12-31T23:59:59Z"` // fin de vigencia (RFC3339)
}

// SwaggerAssignPermissionRequest es el esquema del cuerpo de POST /admin/users/:id/permissions.
// Asigna un permiso directo al usuario, independientemente de sus roles.
type SwaggerAssignPermissionRequest struct {
	PermissionID string  `json:"permission_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	ValidFrom    *string `json:"valid_from,omitempty" example:"2025-01-01T00:00:00Z"`  // inicio de vigencia (RFC3339)
	ValidUntil   *string `json:"valid_until,omitempty" example:"2025-12-31T23:59:59Z"` // fin de vigencia (RFC3339)
}

// SwaggerAssignCostCentersRequest es el esquema del cuerpo de POST /admin/users/:id/cost-centers.
// La operación reemplaza completamente las asignaciones previas del usuario.
type SwaggerAssignCostCentersRequest struct {
	CostCenterIDs []string `json:"cost_center_ids" example:"550e8400-e29b-41d4-a716-446655440000"` // lista de IDs de centros de costo
	ValidFrom     *string  `json:"valid_from,omitempty" example:"2025-01-01T00:00:00Z"`
	ValidUntil    *string  `json:"valid_until,omitempty" example:"2025-12-31T23:59:59Z"`
}

// SwaggerCreateRoleRequest es el esquema del cuerpo de POST /admin/roles.
type SwaggerCreateRoleRequest struct {
	Name        string `json:"name" example:"Supervisor"`
	Description string `json:"description,omitempty" example:"Rol de supervisor de operaciones"` // opcional
}

// SwaggerUpdateRoleRequest es el esquema del cuerpo de PUT /admin/roles/:id.
// Todos los campos son opcionales.
type SwaggerUpdateRoleRequest struct {
	Name        string `json:"name,omitempty" example:"Supervisor Senior"`
	Description string `json:"description,omitempty" example:"Descripción actualizada"`
}

// SwaggerAddRolePermissionRequest es el esquema del cuerpo de POST /admin/roles/:id/permissions.
// Permite asignar múltiples permisos al rol en una sola petición.
type SwaggerAddRolePermissionRequest struct {
	PermissionIDs []string `json:"permission_ids" example:"550e8400-e29b-41d4-a716-446655440000"` // lista de IDs de permisos
}

// SwaggerCreatePermissionRequest es el esquema del cuerpo de POST /admin/permissions.
// scope_type define el alcance del permiso dentro del sistema RBAC.
type SwaggerCreatePermissionRequest struct {
	Code        string `json:"code" example:"admin.reportes.read"`
	Description string `json:"description,omitempty" example:"Lectura de reportes administrativos"` // opcional
	ScopeType   string `json:"scope_type" enums:"global,module,resource,action" example:"module"`   // enum: global | module | resource | action
}

// SwaggerCreateCostCenterRequest es el esquema del cuerpo de POST /admin/cost-centers.
// El campo code es el identificador legible y único dentro de la aplicación.
type SwaggerCreateCostCenterRequest struct {
	Code string `json:"code" example:"CC-001"` // código único legible (ej. "CC-001", "RRHH")
	Name string `json:"name" example:"Centro de Costo Operaciones"`
}

// SwaggerUpdateCostCenterRequest es el esquema del cuerpo de PUT /admin/cost-centers/:id.
// El código del centro de costo no puede modificarse después de la creación.
type SwaggerUpdateCostCenterRequest struct {
	Name     string `json:"name,omitempty" example:"Operaciones Actualizado"`
	IsActive bool   `json:"is_active" example:"true"` // por defecto true; false desactiva el centro de costo
}

// SwaggerCreateApplicationRequest es el esquema del cuerpo de POST /admin/applications.
// El slug debe seguir el formato kebab-case (ej. "mi-app", "portal-rrhh").
type SwaggerCreateApplicationRequest struct {
	Name string `json:"name" example:"Mi Aplicación"`
	Slug string `json:"slug" example:"mi-aplicacion"` // formato: letras minúsculas, dígitos y guiones
}

// SwaggerUpdateApplicationRequest es el esquema del cuerpo de PUT /admin/applications/:id.
// El slug no puede modificarse después de la creación.
type SwaggerUpdateApplicationRequest struct {
	Name     string `json:"name,omitempty" example:"Mi Aplicación Actualizada"`
	IsActive *bool  `json:"is_active,omitempty" example:"true"` // opcional, activar/desactivar aplicación
}

// ─── RESPONSES ─────────────────────────────────────────────────────────────────

// SwaggerErrorDetail es el detalle interno del error en la respuesta estándar.
// El campo code es un identificador de máquina (ej. "INVALID_CREDENTIALS")
// y message es un texto legible para mostrar al usuario o registrar en logs.
type SwaggerErrorDetail struct {
	Code    string      `json:"code" example:"VALIDATION_ERROR"`    // código de máquina del error
	Message string      `json:"message" example:"campo requerido"`  // descripción legible del error
	Details interface{} `json:"details"`                            // reservado para lista de campos inválidos (actualmente null)
}

// SwaggerErrorResponse es la respuesta de error estándar de la API.
// Todos los endpoints la usan cuando retornan un código >= 400.
type SwaggerErrorResponse struct {
	Error SwaggerErrorDetail `json:"error"`
}

// SwaggerHealthChecks contiene el estado de cada servicio de infraestructura
// verificado por el endpoint GET /health.
type SwaggerHealthChecks struct {
	PostgreSQL string `json:"postgresql" example:"ok"` // estado de la conexión a PostgreSQL: "ok" o mensaje de error
	Redis      string `json:"redis" example:"ok"`      // estado de la conexión a Redis: "ok" o mensaje de error
}

// SwaggerHealthResponse es la respuesta de GET /health.
// El campo status es "healthy" cuando todos los checks pasan, "degraded" si alguno falla.
type SwaggerHealthResponse struct {
	Status  string              `json:"status" example:"healthy"`  // "healthy" o "degraded"
	Version string              `json:"version" example:"1.0.0"`
	Checks  SwaggerHealthChecks `json:"checks"`
}

// SwaggerLoginUser contiene los datos del usuario incluidos en la respuesta de login.
// Si must_change_password es true, el frontend debe mostrar el diálogo de cambio
// de contraseña antes de permitir la navegación normal.
type SwaggerLoginUser struct {
	ID                 string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Username           string `json:"username" example:"admin"`
	Email              string `json:"email" example:"admin@empresa.com"`
	MustChangePassword bool   `json:"must_change_password" example:"false"` // true si el admin hizo reset de contraseña
}

// SwaggerLoginResponse es la respuesta exitosa de POST /auth/login.
// El access_token es un JWT RS256 de corta duración (generalmente 1h).
// El refresh_token es un UUID v4 de larga duración para renovar el access_token.
type SwaggerLoginResponse struct {
	AccessToken  string           `json:"access_token" example:"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string           `json:"refresh_token" example:"550e8400-e29b-41d4-a716-446655440000"` // UUID v4 raw
	TokenType    string           `json:"token_type" example:"Bearer"`
	ExpiresIn    int              `json:"expires_in" example:"3600"` // segundos hasta la expiración del access_token
	User         SwaggerLoginUser `json:"user"`
}

// SwaggerTokenResponse es la respuesta exitosa de POST /auth/refresh.
// El nuevo par de tokens reemplaza completamente al anterior.
type SwaggerTokenResponse struct {
	AccessToken  string `json:"access_token" example:"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string `json:"refresh_token" example:"550e8400-e29b-41d4-a716-446655440000"` // nuevo UUID v4
	TokenType    string `json:"token_type" example:"Bearer"`
	ExpiresIn    int    `json:"expires_in" example:"3600"` // segundos hasta la expiración del access_token
}

// SwaggerVerifyResponse es la respuesta de POST /authz/verify.
// El campo allowed indica si el usuario tiene el permiso solicitado.
// evaluated_at permite a los backends registrar cuándo se evaluó el permiso.
type SwaggerVerifyResponse struct {
	Allowed     bool   `json:"allowed" example:"true"`                                        // true si el usuario tiene el permiso
	UserID      string `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Username    string `json:"username" example:"admin"`
	Permission  string `json:"permission" example:"admin.users.read"`
	EvaluatedAt string `json:"evaluated_at" example:"2025-01-15T10:30:00Z"` // timestamp de la evaluación en UTC
}

// SwaggerMePermissionsResponse es la respuesta de GET /authz/me/permissions.
// Contiene el perfil de autorización completo del usuario para la aplicación actual.
type SwaggerMePermissionsResponse struct {
	UserID         string   `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Application    string   `json:"application" example:"sentinel"`                             // slug de la aplicación
	Roles          []string `json:"roles"`                                                      // roles activos del usuario
	Permissions    []string `json:"permissions"`                                                // permisos efectivos (roles + directos)
	CostCenters    []string `json:"cost_centers"`                                               // centros de costo asignados
	TemporaryRoles []string `json:"temporary_roles"`                                            // roles con vigencia temporal activa
}

// SwaggerPermissionsMapVersionResponse es la respuesta de GET /authz/permissions-map/version.
// Permite a los backends verificar si su mapa cacheado está desactualizado.
type SwaggerPermissionsMapVersionResponse struct {
	Application string `json:"application" example:"sentinel"`             // slug de la aplicación
	Version     string `json:"version" example:"abc123def456"`             // hash SHA256 del mapa canónico
	GeneratedAt string `json:"generated_at" example:"2025-01-15T10:30:00Z"` // timestamp de última generación
}

// SwaggerResetPasswordResponse es la respuesta de POST /admin/users/:id/reset-password.
// La contraseña temporal se entrega en texto plano una única vez; debe comunicarse
// al usuario por un canal seguro y cambiarse en el próximo login.
type SwaggerResetPasswordResponse struct {
	TemporaryPassword string `json:"temporary_password" example:"TempPass@2025!"` // contraseña temporal de un solo uso
}

// SwaggerRotateKeyResponse es la respuesta de POST /admin/applications/:id/rotate-key.
// La nueva clave secreta se entrega en texto plano una única vez; guardarla antes
// de cerrar esta respuesta.
type SwaggerRotateKeyResponse struct {
	SecretKey string `json:"secret_key" example:"abc123def456ghi789jkl012mno345pqr678stu901vwx234yz"` // nueva clave en Base64 URL-safe
}

// SwaggerAssignedCountResponse es la respuesta genérica para operaciones de
// asignación masiva (roles, permisos, centros de costo). Indica cuántos
// elementos fueron procesados en la operación.
type SwaggerAssignedCountResponse struct {
	Assigned int `json:"assigned" example:"3"` // número de elementos asignados en la operación
}

// ─── PAGINACIÓN ────────────────────────────────────────────────────────────────

// SwaggerUserItem representa un elemento de usuario en la respuesta paginada.
// Los campos FailedAttempts y LockedUntil son útiles para diagnóstico de acceso.
type SwaggerUserItem struct {
	ID             string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Username       string  `json:"username" example:"jdoe"`
	Email          string  `json:"email" example:"jdoe@empresa.com"`
	IsActive       bool    `json:"is_active" example:"true"`
	MustChangePwd  bool    `json:"must_change_pwd" example:"false"`             // true si requiere cambio de contraseña al próximo login
	LastLoginAt    *string `json:"last_login_at" example:"2025-01-15T10:30:00Z"` // null si nunca ha iniciado sesión
	FailedAttempts int     `json:"failed_attempts" example:"0"`                 // contador de intentos fallidos consecutivos
	LockedUntil    *string `json:"locked_until"`                                // null si no está bloqueado temporalmente
	CreatedAt      string  `json:"created_at" example:"2025-01-01T00:00:00Z"`
}

// SwaggerPaginatedUsers es la respuesta paginada del listado de usuarios.
type SwaggerPaginatedUsers struct {
	Data       []SwaggerUserItem `json:"data"`
	Page       int               `json:"page" example:"1"`
	PageSize   int               `json:"page_size" example:"20"`
	Total      int               `json:"total" example:"42"`        // total de registros que cumplen los filtros
	TotalPages int               `json:"total_pages" example:"3"` // calculado: ceil(total / page_size)
}

// SwaggerRoleItem representa un elemento de rol en la respuesta paginada.
// IsSystem=true indica que el rol fue creado por el bootstrap y no puede eliminarse.
type SwaggerRoleItem struct {
	ID          string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name        string  `json:"name" example:"Administrador"`
	Description *string `json:"description" example:"Rol de administrador del sistema"` // null si no tiene descripción
	IsSystem    bool    `json:"is_system" example:"true"`                               // true para roles predefinidos del sistema
	IsActive    bool    `json:"is_active" example:"true"`
	CreatedAt   string  `json:"created_at" example:"2025-01-01T00:00:00Z"`
}

// SwaggerPaginatedRoles es la respuesta paginada del listado de roles.
type SwaggerPaginatedRoles struct {
	Data       []SwaggerRoleItem `json:"data"`
	Page       int               `json:"page" example:"1"`
	PageSize   int               `json:"page_size" example:"20"`
	Total      int               `json:"total" example:"5"`
	TotalPages int               `json:"total_pages" example:"1"`
}

// SwaggerPermissionItem representa un elemento de permiso en la respuesta paginada.
// El código sigue el formato punteado: "<app>.<modulo>.<accion>".
type SwaggerPermissionItem struct {
	ID          string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Code        string  `json:"code" example:"admin.users.read"`                  // código único del permiso
	Description *string `json:"description" example:"Lectura de usuarios"`        // null si no tiene descripción
	ScopeType   string  `json:"scope_type" example:"module"`                      // global | module | resource | action
	CreatedAt   string  `json:"created_at" example:"2025-01-01T00:00:00Z"`
}

// SwaggerPaginatedPermissions es la respuesta paginada del listado de permisos.
type SwaggerPaginatedPermissions struct {
	Data       []SwaggerPermissionItem `json:"data"`
	Page       int                     `json:"page" example:"1"`
	PageSize   int                     `json:"page_size" example:"20"`
	Total      int                     `json:"total" example:"15"`
	TotalPages int                     `json:"total_pages" example:"1"`
}

// SwaggerCostCenterItem representa un elemento de centro de costo en la respuesta paginada.
type SwaggerCostCenterItem struct {
	ID        string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Code      string `json:"code" example:"CC-001"`                             // código legible único dentro de la aplicación
	Name      string `json:"name" example:"Centro de Costo Operaciones"`
	IsActive  bool   `json:"is_active" example:"true"`
	CreatedAt string `json:"created_at" example:"2025-01-01T00:00:00Z"`
}

// SwaggerPaginatedCostCenters es la respuesta paginada del listado de centros de costo.
type SwaggerPaginatedCostCenters struct {
	Data       []SwaggerCostCenterItem `json:"data"`
	Page       int                     `json:"page" example:"1"`
	PageSize   int                     `json:"page_size" example:"20"`
	Total      int                     `json:"total" example:"8"`
	TotalPages int                     `json:"total_pages" example:"1"`
}

// SwaggerApplicationItem representa un elemento de aplicación en la respuesta paginada.
// IsSystem=true indica la aplicación interna del sistema que no puede modificarse.
// La clave secreta no se incluye en el listado; solo en GetApplication y al crear/rotar.
type SwaggerApplicationItem struct {
	ID        string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name      string `json:"name" example:"Mi Aplicación"`
	Slug      string `json:"slug" example:"mi-aplicacion"`                     // identificador inmutable en kebab-case
	IsActive  bool   `json:"is_active" example:"true"`
	IsSystem  bool   `json:"is_system" example:"false"`                        // true solo para la app con slug="system"
	CreatedAt string `json:"created_at" example:"2025-01-01T00:00:00Z"`
	UpdatedAt string `json:"updated_at" example:"2025-01-15T10:30:00Z"`
}

// SwaggerPaginatedApplications es la respuesta paginada del listado de aplicaciones.
type SwaggerPaginatedApplications struct {
	Data       []SwaggerApplicationItem `json:"data"`
	Page       int                      `json:"page" example:"1"`
	PageSize   int                      `json:"page_size" example:"20"`
	Total      int                      `json:"total" example:"3"`
	TotalPages int                      `json:"total_pages" example:"1"`
}

// SwaggerAuditLogItem representa un evento de auditoría en la respuesta paginada.
// Los campos UserID, ActorID y ApplicationID son opcionales porque algunos eventos
// del sistema se generan sin un usuario o aplicación específicos.
type SwaggerAuditLogItem struct {
	ID            string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	EventType     string  `json:"event_type" example:"LOGIN_SUCCESS"`                     // tipo de evento (ej: LOGIN_SUCCESS, ROLE_ASSIGNED)
	UserID        *string `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"` // usuario afectado por el evento (puede ser null)
	ActorID       *string `json:"actor_id" example:"550e8400-e29b-41d4-a716-446655440000"` // usuario que ejecutó la acción (puede ser null para eventos del sistema)
	ApplicationID *string `json:"application_id" example:"550e8400-e29b-41d4-a716-446655440000"` // aplicación cliente del evento (puede ser null)
	IPAddress     string  `json:"ip_address" example:"192.168.1.100"`                     // IP de origen de la petición
	Success       bool    `json:"success" example:"true"`                                 // true si el evento fue exitoso
	CreatedAt     string  `json:"created_at" example:"2025-01-15T10:30:00Z"`
}

// SwaggerPaginatedAuditLogs es la respuesta paginada del listado de logs de auditoría.
type SwaggerPaginatedAuditLogs struct {
	Data       []SwaggerAuditLogItem `json:"data"`
	Page       int                   `json:"page" example:"1"`
	PageSize   int                   `json:"page_size" example:"20"`
	Total      int                   `json:"total" example:"100"`
	TotalPages int                   `json:"total_pages" example:"5"`
}

// ─── TIPOS ESPECÍFICOS ──────────────────────────────────────────────────────

// SwaggerJWKSKey representa una clave pública RSA en formato JWKS (RFC 7517).
// Los backends consumidores usan este objeto para verificar localmente los
// tokens JWT RS256 sin necesidad de contactar a Sentinel en cada petición.
type SwaggerJWKSKey struct {
	Kty string `json:"kty" example:"RSA"`             // tipo de clave: siempre "RSA"
	Alg string `json:"alg" example:"RS256"`            // algoritmo: siempre "RS256"
	Use string `json:"use" example:"sig"`              // uso: siempre "sig" (firma)
	Kid string `json:"kid" example:"2026-02-key-01"`  // ID de la clave para selección en rotación
	N   string `json:"n" example:"sF3eLJzG..."`       // módulo RSA en Base64 URL-safe
	E   string `json:"e" example:"AQAB"`              // exponente público RSA en Base64 URL-safe
}

// SwaggerJWKSResponse es la respuesta de GET /.well-known/jwks.json.
// El array "keys" puede contener múltiples claves durante períodos de rotación.
type SwaggerJWKSResponse struct {
	Keys []SwaggerJWKSKey `json:"keys"` // conjunto de claves públicas RSA activas
}

// SwaggerRoleDetailResponse es la respuesta de GET /admin/roles/:id.
// Incluye la lista completa de permisos asignados al rol y el número de usuarios activos.
type SwaggerRoleDetailResponse struct {
	ID          string                  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name        string                  `json:"name" example:"Administrador"`
	Description *string                 `json:"description" example:"Rol de administrador del sistema"` // null si no tiene descripción
	IsSystem    bool                    `json:"is_system" example:"true"`
	IsActive    bool                    `json:"is_active" example:"true"`
	Permissions []SwaggerPermissionItem `json:"permissions"`      // lista de permisos asignados al rol
	UsersCount  int                     `json:"users_count" example:"5"` // número de usuarios con este rol activo
	CreatedAt   string                  `json:"created_at" example:"2025-01-01T00:00:00Z"`
	UpdatedAt   string                  `json:"updated_at" example:"2025-01-15T10:30:00Z"`
}

// SwaggerAssignedRoleResponse es la respuesta de POST /admin/users/:id/roles.
// Representa el registro de asignación creado en la tabla user_roles.
type SwaggerAssignedRoleResponse struct {
	ID         string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`         // ID del registro de asignación
	UserID     string  `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	RoleID     string  `json:"role_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	RoleName   string  `json:"role_name" example:"Administrador"`
	ValidFrom  string  `json:"valid_from" example:"2025-01-01T00:00:00Z"`
	ValidUntil *string `json:"valid_until" example:"2025-12-31T23:59:59Z"` // null si la asignación es permanente
	GrantedBy  string  `json:"granted_by" example:"550e8400-e29b-41d4-a716-446655440000"` // ID del admin que realizó la asignación
}

// SwaggerAssignedPermissionResponse es la respuesta de POST /admin/users/:id/permissions.
// Representa el registro de asignación directa creado en la tabla user_permissions.
type SwaggerAssignedPermissionResponse struct {
	ID           string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`            // ID del registro de asignación
	UserID       string  `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	PermissionID string  `json:"permission_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	ValidFrom    string  `json:"valid_from" example:"2025-01-01T00:00:00Z"`
	ValidUntil   *string `json:"valid_until" example:"2025-12-31T23:59:59Z"` // null si la asignación es permanente
	GrantedBy    string  `json:"granted_by" example:"550e8400-e29b-41d4-a716-446655440000"` // ID del admin que realizó la asignación
}

// SwaggerPermissionMapEntry es una entrada en el mapa de permisos de una aplicación.
// La clave del mapa es el código del permiso (ej. "erp.reportes.read").
type SwaggerPermissionMapEntry struct {
	Roles       []string `json:"roles"`                                             // roles que tienen este permiso asignado
	Description string   `json:"description" example:"Lectura de usuarios"`
}

// SwaggerCostCenterMapEntry es una entrada de centro de costo en el mapa de permisos.
// La clave del mapa es el ID del centro de costo.
type SwaggerCostCenterMapEntry struct {
	Code     string `json:"code" example:"CC-001"`
	Name     string `json:"name" example:"Centro de Costo Operaciones"`
	IsActive bool   `json:"is_active" example:"true"`
}

// SwaggerPermissionsMapResponse es la respuesta de GET /authz/permissions-map.
// El campo signature contiene la firma RSA-SHA256 del JSON canónico del mapa
// (claves ordenadas lexicográficamente, sin espacios, codificada en Base64 URL-safe).
type SwaggerPermissionsMapResponse struct {
	Application string                               `json:"application" example:"sentinel"`              // slug de la aplicación
	GeneratedAt string                               `json:"generated_at" example:"2025-01-15T10:30:00Z"` // timestamp de generación
	Version     string                               `json:"version" example:"a1b2c3d4"`                  // hash SHA256 del mapa canónico
	Permissions map[string]SwaggerPermissionMapEntry `json:"permissions"`                                 // mapa codigo->entrada
	CostCenters map[string]SwaggerCostCenterMapEntry `json:"cost_centers"`                                // mapa id->entrada
	Signature   string                               `json:"signature" example:"base64url-encoded-rsa-sha256-signature"` // firma RSA-SHA256
}
