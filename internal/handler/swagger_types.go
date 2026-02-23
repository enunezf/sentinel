package handler

// Este archivo define structs utilizados exclusivamente para las anotaciones de Swagger.
// Los handlers internamente usan fiber.Map{}; estos tipos solo sirven para que swag
// genere los JSON schemas correctamente en la documentación.

// ─── REQUESTS ──────────────────────────────────────────────────────────────────

// SwaggerLoginRequest es el cuerpo de POST /auth/login.
type SwaggerLoginRequest struct {
	Username   string `json:"username" example:"admin"`
	Password   string `json:"password" example:"Admin@Local1!"`
	ClientType string `json:"client_type" enums:"web,mobile,desktop" example:"web"`
}

// SwaggerRefreshRequest es el cuerpo de POST /auth/refresh.
type SwaggerRefreshRequest struct {
	RefreshToken string `json:"refresh_token" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// SwaggerChangePasswordRequest es el cuerpo de POST /auth/change-password.
type SwaggerChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" example:"Admin@Local1!"`
	NewPassword     string `json:"new_password" example:"NuevaClave@2025!"`
}

// SwaggerVerifyPermissionRequest es el cuerpo de POST /authz/verify.
type SwaggerVerifyPermissionRequest struct {
	Permission   string `json:"permission" example:"admin.users.read"`
	CostCenterID string `json:"cost_center_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// SwaggerCreateUserRequest es el cuerpo de POST /admin/users.
type SwaggerCreateUserRequest struct {
	Username string `json:"username" example:"jdoe"`
	Email    string `json:"email" example:"jdoe@empresa.com"`
	Password string `json:"password" example:"Clave@Segura1!"`
}

// SwaggerUpdateUserRequest es el cuerpo de PUT /admin/users/:id.
type SwaggerUpdateUserRequest struct {
	Username *string `json:"username,omitempty" example:"jdoe_nuevo"`
	Email    *string `json:"email,omitempty" example:"nuevo@empresa.com"`
	IsActive *bool   `json:"is_active,omitempty" example:"true"`
}

// SwaggerAssignRoleRequest es el cuerpo de POST /admin/users/:id/roles.
type SwaggerAssignRoleRequest struct {
	RoleID     string  `json:"role_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	ValidFrom  *string `json:"valid_from,omitempty" example:"2025-01-01T00:00:00Z"`
	ValidUntil *string `json:"valid_until,omitempty" example:"2025-12-31T23:59:59Z"`
}

// SwaggerAssignPermissionRequest es el cuerpo de POST /admin/users/:id/permissions.
type SwaggerAssignPermissionRequest struct {
	PermissionID string  `json:"permission_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	ValidFrom    *string `json:"valid_from,omitempty" example:"2025-01-01T00:00:00Z"`
	ValidUntil   *string `json:"valid_until,omitempty" example:"2025-12-31T23:59:59Z"`
}

// SwaggerAssignCostCentersRequest es el cuerpo de POST /admin/users/:id/cost-centers.
type SwaggerAssignCostCentersRequest struct {
	CostCenterIDs []string `json:"cost_center_ids" example:"550e8400-e29b-41d4-a716-446655440000"`
	ValidFrom     *string  `json:"valid_from,omitempty" example:"2025-01-01T00:00:00Z"`
	ValidUntil    *string  `json:"valid_until,omitempty" example:"2025-12-31T23:59:59Z"`
}

// SwaggerCreateRoleRequest es el cuerpo de POST /admin/roles.
type SwaggerCreateRoleRequest struct {
	Name        string `json:"name" example:"Supervisor"`
	Description string `json:"description,omitempty" example:"Rol de supervisor de operaciones"`
}

// SwaggerUpdateRoleRequest es el cuerpo de PUT /admin/roles/:id.
type SwaggerUpdateRoleRequest struct {
	Name        string `json:"name,omitempty" example:"Supervisor Senior"`
	Description string `json:"description,omitempty" example:"Descripción actualizada"`
}

// SwaggerAddRolePermissionRequest es el cuerpo de POST /admin/roles/:id/permissions.
type SwaggerAddRolePermissionRequest struct {
	PermissionIDs []string `json:"permission_ids" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// SwaggerCreatePermissionRequest es el cuerpo de POST /admin/permissions.
type SwaggerCreatePermissionRequest struct {
	Code        string `json:"code" example:"admin.reportes.read"`
	Description string `json:"description,omitempty" example:"Lectura de reportes administrativos"`
	ScopeType   string `json:"scope_type" enums:"global,module,resource,action" example:"module"`
}

// SwaggerCreateCostCenterRequest es el cuerpo de POST /admin/cost-centers.
type SwaggerCreateCostCenterRequest struct {
	Code string `json:"code" example:"CC-001"`
	Name string `json:"name" example:"Centro de Costo Operaciones"`
}

// SwaggerUpdateCostCenterRequest es el cuerpo de PUT /admin/cost-centers/:id.
type SwaggerUpdateCostCenterRequest struct {
	Name     string `json:"name,omitempty" example:"Operaciones Actualizado"`
	IsActive bool   `json:"is_active" example:"true"`
}

// SwaggerCreateApplicationRequest es el cuerpo de POST /admin/applications.
type SwaggerCreateApplicationRequest struct {
	Name string `json:"name" example:"Mi Aplicación"`
	Slug string `json:"slug" example:"mi-aplicacion"`
}

// SwaggerUpdateApplicationRequest es el cuerpo de PUT /admin/applications/:id.
type SwaggerUpdateApplicationRequest struct {
	Name     string `json:"name,omitempty" example:"Mi Aplicación Actualizada"`
	IsActive *bool  `json:"is_active,omitempty" example:"true"`
}

// ─── RESPONSES ─────────────────────────────────────────────────────────────────

// SwaggerErrorDetail detalle de un error de la API.
type SwaggerErrorDetail struct {
	Code    string      `json:"code" example:"VALIDATION_ERROR"`
	Message string      `json:"message" example:"campo requerido"`
	Details interface{} `json:"details"`
}

// SwaggerErrorResponse es la respuesta de error estándar.
type SwaggerErrorResponse struct {
	Error SwaggerErrorDetail `json:"error"`
}

// SwaggerHealthChecks estado de los servicios de infraestructura.
type SwaggerHealthChecks struct {
	PostgreSQL string `json:"postgresql" example:"ok"`
	Redis      string `json:"redis" example:"ok"`
}

// SwaggerHealthResponse es la respuesta de GET /health.
type SwaggerHealthResponse struct {
	Status  string              `json:"status" example:"healthy"`
	Version string              `json:"version" example:"1.0.0"`
	Checks  SwaggerHealthChecks `json:"checks"`
}

// SwaggerLoginUser información del usuario en la respuesta de login.
type SwaggerLoginUser struct {
	ID                 string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Username           string `json:"username" example:"admin"`
	Email              string `json:"email" example:"admin@empresa.com"`
	MustChangePassword bool   `json:"must_change_password" example:"false"`
}

// SwaggerLoginResponse es la respuesta de POST /auth/login.
type SwaggerLoginResponse struct {
	AccessToken  string           `json:"access_token" example:"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string           `json:"refresh_token" example:"550e8400-e29b-41d4-a716-446655440000"`
	TokenType    string           `json:"token_type" example:"Bearer"`
	ExpiresIn    int              `json:"expires_in" example:"3600"`
	User         SwaggerLoginUser `json:"user"`
}

// SwaggerTokenResponse es la respuesta de POST /auth/refresh.
type SwaggerTokenResponse struct {
	AccessToken  string `json:"access_token" example:"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string `json:"refresh_token" example:"550e8400-e29b-41d4-a716-446655440000"`
	TokenType    string `json:"token_type" example:"Bearer"`
	ExpiresIn    int    `json:"expires_in" example:"3600"`
}

// SwaggerVerifyResponse es la respuesta de POST /authz/verify.
type SwaggerVerifyResponse struct {
	Allowed     bool   `json:"allowed" example:"true"`
	UserID      string `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Username    string `json:"username" example:"admin"`
	Permission  string `json:"permission" example:"admin.users.read"`
	EvaluatedAt string `json:"evaluated_at" example:"2025-01-15T10:30:00Z"`
}

// SwaggerMePermissionsResponse es la respuesta de GET /authz/me/permissions.
type SwaggerMePermissionsResponse struct {
	UserID         string   `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Application    string   `json:"application" example:"sentinel"`
	Roles          []string `json:"roles"`
	Permissions    []string `json:"permissions"`
	CostCenters    []string `json:"cost_centers"`
	TemporaryRoles []string `json:"temporary_roles"`
}

// SwaggerPermissionsMapVersionResponse es la respuesta de GET /authz/permissions-map/version.
type SwaggerPermissionsMapVersionResponse struct {
	Application string `json:"application" example:"sentinel"`
	Version     string `json:"version" example:"abc123def456"`
	GeneratedAt string `json:"generated_at" example:"2025-01-15T10:30:00Z"`
}

// SwaggerResetPasswordResponse es la respuesta de POST /admin/users/:id/reset-password.
type SwaggerResetPasswordResponse struct {
	TemporaryPassword string `json:"temporary_password" example:"TempPass@2025!"`
}

// SwaggerRotateKeyResponse es la respuesta de POST /admin/applications/:id/rotate-key.
type SwaggerRotateKeyResponse struct {
	SecretKey string `json:"secret_key" example:"abc123def456ghi789jkl012mno345pqr678stu901vwx234yz"`
}

// SwaggerAssignedCountResponse respuesta genérica de conteo de elementos asignados.
type SwaggerAssignedCountResponse struct {
	Assigned int `json:"assigned" example:"3"`
}

// ─── PAGINACIÓN ────────────────────────────────────────────────────────────────

// SwaggerUserItem representa un usuario en la lista paginada.
type SwaggerUserItem struct {
	ID             string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Username       string  `json:"username" example:"jdoe"`
	Email          string  `json:"email" example:"jdoe@empresa.com"`
	IsActive       bool    `json:"is_active" example:"true"`
	MustChangePwd  bool    `json:"must_change_pwd" example:"false"`
	LastLoginAt    *string `json:"last_login_at" example:"2025-01-15T10:30:00Z"`
	FailedAttempts int     `json:"failed_attempts" example:"0"`
	LockedUntil    *string `json:"locked_until"`
	CreatedAt      string  `json:"created_at" example:"2025-01-01T00:00:00Z"`
}

// SwaggerPaginatedUsers respuesta paginada de usuarios.
type SwaggerPaginatedUsers struct {
	Data       []SwaggerUserItem `json:"data"`
	Page       int               `json:"page" example:"1"`
	PageSize   int               `json:"page_size" example:"20"`
	Total      int               `json:"total" example:"42"`
	TotalPages int               `json:"total_pages" example:"3"`
}

// SwaggerRoleItem representa un rol en la lista paginada.
type SwaggerRoleItem struct {
	ID          string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name        string  `json:"name" example:"Administrador"`
	Description *string `json:"description" example:"Rol de administrador del sistema"`
	IsSystem    bool    `json:"is_system" example:"true"`
	IsActive    bool    `json:"is_active" example:"true"`
	CreatedAt   string  `json:"created_at" example:"2025-01-01T00:00:00Z"`
}

// SwaggerPaginatedRoles respuesta paginada de roles.
type SwaggerPaginatedRoles struct {
	Data       []SwaggerRoleItem `json:"data"`
	Page       int               `json:"page" example:"1"`
	PageSize   int               `json:"page_size" example:"20"`
	Total      int               `json:"total" example:"5"`
	TotalPages int               `json:"total_pages" example:"1"`
}

// SwaggerPermissionItem representa un permiso en la lista paginada.
type SwaggerPermissionItem struct {
	ID          string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Code        string  `json:"code" example:"admin.users.read"`
	Description *string `json:"description" example:"Lectura de usuarios"`
	ScopeType   string  `json:"scope_type" example:"module"`
	CreatedAt   string  `json:"created_at" example:"2025-01-01T00:00:00Z"`
}

// SwaggerPaginatedPermissions respuesta paginada de permisos.
type SwaggerPaginatedPermissions struct {
	Data       []SwaggerPermissionItem `json:"data"`
	Page       int                     `json:"page" example:"1"`
	PageSize   int                     `json:"page_size" example:"20"`
	Total      int                     `json:"total" example:"15"`
	TotalPages int                     `json:"total_pages" example:"1"`
}

// SwaggerCostCenterItem representa un centro de costo en la lista paginada.
type SwaggerCostCenterItem struct {
	ID        string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Code      string `json:"code" example:"CC-001"`
	Name      string `json:"name" example:"Centro de Costo Operaciones"`
	IsActive  bool   `json:"is_active" example:"true"`
	CreatedAt string `json:"created_at" example:"2025-01-01T00:00:00Z"`
}

// SwaggerPaginatedCostCenters respuesta paginada de centros de costo.
type SwaggerPaginatedCostCenters struct {
	Data       []SwaggerCostCenterItem `json:"data"`
	Page       int                     `json:"page" example:"1"`
	PageSize   int                     `json:"page_size" example:"20"`
	Total      int                     `json:"total" example:"8"`
	TotalPages int                     `json:"total_pages" example:"1"`
}

// SwaggerApplicationItem representa una aplicación en la lista paginada.
type SwaggerApplicationItem struct {
	ID        string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name      string `json:"name" example:"Mi Aplicación"`
	Slug      string `json:"slug" example:"mi-aplicacion"`
	IsActive  bool   `json:"is_active" example:"true"`
	IsSystem  bool   `json:"is_system" example:"false"`
	CreatedAt string `json:"created_at" example:"2025-01-01T00:00:00Z"`
	UpdatedAt string `json:"updated_at" example:"2025-01-15T10:30:00Z"`
}

// SwaggerPaginatedApplications respuesta paginada de aplicaciones.
type SwaggerPaginatedApplications struct {
	Data       []SwaggerApplicationItem `json:"data"`
	Page       int                      `json:"page" example:"1"`
	PageSize   int                      `json:"page_size" example:"20"`
	Total      int                      `json:"total" example:"3"`
	TotalPages int                      `json:"total_pages" example:"1"`
}

// SwaggerAuditLogItem representa un registro de auditoría en la lista paginada.
type SwaggerAuditLogItem struct {
	ID            string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	EventType     string  `json:"event_type" example:"LOGIN_SUCCESS"`
	UserID        *string `json:"user_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	ActorID       *string `json:"actor_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	ApplicationID *string `json:"application_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	IPAddress     string  `json:"ip_address" example:"192.168.1.100"`
	Success       bool    `json:"success" example:"true"`
	CreatedAt     string  `json:"created_at" example:"2025-01-15T10:30:00Z"`
}

// SwaggerPaginatedAuditLogs respuesta paginada de registros de auditoría.
type SwaggerPaginatedAuditLogs struct {
	Data       []SwaggerAuditLogItem `json:"data"`
	Page       int                   `json:"page" example:"1"`
	PageSize   int                   `json:"page_size" example:"20"`
	Total      int                   `json:"total" example:"100"`
	TotalPages int                   `json:"total_pages" example:"5"`
}
