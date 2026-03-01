package handler

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/middleware"
	"github.com/enunezf/sentinel/internal/repository/postgres"
	"github.com/enunezf/sentinel/internal/service"
)

// slugRegexp valida que el slug de una aplicación siga el formato kebab-case:
// solo letras minúsculas, dígitos y guiones, sin guiones al inicio ni al final
// (ej. "mi-app", "erp2", "portal-rrhh"). Esto garantiza compatibilidad con
// URLs y nombres de subdominios.
var slugRegexp = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// AdminHandler agrupa todos los handlers del área de administración (/admin/*).
// Cubre la gestión de usuarios, roles, permisos, centros de costo, aplicaciones
// y la consulta de logs de auditoría.
// Todos sus endpoints requieren JWT válido y el permiso RBAC correspondiente,
// verificados por los middlewares JWTAuth y RequirePermission en el router.
type AdminHandler struct {
	userSvc   *service.UserService              // lógica de negocio de usuarios (crear, actualizar, asignar roles/permisos)
	roleSvc   *service.RoleService              // lógica de negocio de roles (CRUD, asignación de permisos)
	permSvc   *service.PermissionService        // lógica de negocio de permisos (CRUD)
	ccSvc     *service.CostCenterService        // lógica de negocio de centros de costo
	auditRepo *postgres.AuditRepository         // acceso directo al repositorio de auditoría para consultas
	appRepo   *postgres.ApplicationRepository   // acceso directo al repositorio de aplicaciones cliente
	logger    *slog.Logger                      // logger estructurado con atributo component="admin"
}

// NewAdminHandler construye un AdminHandler inyectando sus dependencias.
// El logger recibido se enriquece con el atributo component="admin".
func NewAdminHandler(
	userSvc *service.UserService,
	roleSvc *service.RoleService,
	permSvc *service.PermissionService,
	ccSvc *service.CostCenterService,
	auditRepo *postgres.AuditRepository,
	appRepo *postgres.ApplicationRepository,
	log *slog.Logger,
) *AdminHandler {
	return &AdminHandler{
		userSvc:   userSvc,
		roleSvc:   roleSvc,
		permSvc:   permSvc,
		ccSvc:     ccSvc,
		auditRepo: auditRepo,
		appRepo:   appRepo,
		logger:    log.With("component", "admin"),
	}
}

// internalError registra un error inesperado con nivel ERROR y retorna una
// respuesta 500 al cliente. Se utiliza cuando el error no es atribuible al
// usuario (ej. fallo de base de datos). El mensaje técnico se loguea pero
// nunca se expone en la respuesta para no filtrar detalles internos.
func (h *AdminHandler) internalError(c *fiber.Ctx, err error, msg string) error {
	requestID, _ := c.Locals(middleware.LocalRequestID).(string)
	h.logger.Error(msg,
		"error", err,
		"request_id", requestID,
	)
	return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

// parsePagination extrae y normaliza los parámetros de paginación de la query string.
// Valores por defecto: page=1, page_size=20.
// Restricciones: page mínimo 1, page_size entre 1 y 100 (máximo permitido por la API).
// Valores inválidos (no numéricos) se reemplazan silenciosamente por los defaults.
func parsePagination(c *fiber.Ctx) (page, pageSize int) {
	page, _ = strconv.Atoi(c.Query("page", "1"))
	pageSize, _ = strconv.Atoi(c.Query("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return
}

// totalPages calcula el número total de páginas para una colección paginada.
// Usa math.Ceil para que un resto de elementos genere una página extra.
// Retorna 0 si pageSize es 0 para evitar división por cero.
func totalPages(total, pageSize int) int {
	if pageSize == 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

// paginatedResponse construye el envelope JSON estándar para respuestas paginadas.
// El campo "data" contiene los elementos de la página actual.
// Los campos de paginación (page, page_size, total, total_pages) permiten al
// frontend renderizar controles de navegación sin cálculos adicionales.
func paginatedResponse(data interface{}, page, pageSize, total int) fiber.Map {
	return fiber.Map{
		"data":        data,
		"page":        page,
		"page_size":   pageSize,
		"total":       total,
		"total_pages": totalPages(total, pageSize),
	}
}

// ---- USER ENDPOINTS ----

// ListUsers maneja GET /admin/users.
//
// Retorna la lista paginada de todos los usuarios del sistema.
// Soporta filtrado por texto libre (username o email) mediante el parámetro
// "search" y por estado activo/inactivo mediante "is_active".
//
// Códigos HTTP posibles:
//   - 200: lista paginada de usuarios (puede ser lista vacía)
//   - 401: JWT ausente o inválido
//   - 403: usuario autenticado sin permiso admin.users.read
//   - 500: error de base de datos
//
// ListUsers handles GET /admin/users.
//
// @Summary     Listar usuarios
// @Description Retorna la lista paginada de usuarios. Acepta filtros por búsqueda de texto e estado activo.
// @Tags        Usuarios
// @Produce     json
// @Param       X-App-Key      header   string  true   "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true   "Token JWT. Formato: Bearer {token}"
// @Param       page           query    int     false  "Número de página (default: 1)"
// @Param       page_size      query    int     false  "Elementos por página (default: 20, max: 100)"
// @Param       search         query    string  false  "Búsqueda por username o email"
// @Param       is_active      query    bool    false  "Filtrar por estado activo"
// @Success     200  {object}  SwaggerPaginatedUsers   "Lista paginada de usuarios"
// @Failure     401  {object}  SwaggerErrorResponse    "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse    "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users [get]
func (h *AdminHandler) ListUsers(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)
	search := c.Query("search", "")

	var isActive *bool
	if v := c.Query("is_active", ""); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			isActive = &b
		}
	}

	users, total, err := h.userSvc.ListUsers(c.Context(), postgres.UserFilter{
		Search:   search,
		IsActive: isActive,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return h.internalError(c, err, "admin: list users failed")
	}

	data := make([]fiber.Map, 0, len(users))
	for _, u := range users {
		data = append(data, fiber.Map{
			"id":              u.ID,
			"username":        u.Username,
			"email":           u.Email,
			"is_active":       u.IsActive,
			"must_change_pwd": u.MustChangePwd,
			"last_login_at":   u.LastLoginAt,
			"failed_attempts": u.FailedAttempts,
			"locked_until":    u.LockedUntil,
			"created_at":      u.CreatedAt,
		})
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(data, page, pageSize, total))
}

// CreateUser maneja POST /admin/users.
//
// Crea un nuevo usuario en el sistema. La contraseña debe cumplir la política
// de seguridad (longitud mínima, mayúsculas, dígitos, caracteres especiales).
// El campo actorID se extrae de los claims del JWT para registrar quién creó
// el usuario en el log de auditoría.
//
// Códigos HTTP posibles:
//   - 201: usuario creado exitosamente
//   - 400: body inválido, campos faltantes o política de contraseña no cumplida
//   - 401: JWT ausente o inválido
//   - 403: sin permiso admin.users.write
//   - 409: ya existe un usuario con ese username o email
//   - 500: error interno
//
// CreateUser handles POST /admin/users.
//
// @Summary     Crear usuario
// @Description Crea un nuevo usuario en el sistema. La contraseña debe cumplir la política de seguridad.
// @Tags        Usuarios
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                   true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                   true  "Token JWT. Formato: Bearer {token}"
// @Param       body           body     SwaggerCreateUserRequest true  "Datos del nuevo usuario"
// @Success     201  {object}  SwaggerUserItem       "Usuario creado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse  "Datos inválidos o política de contraseña"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users [post]
func (h *AdminHandler) CreateUser(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Username == "" || body.Email == "" || body.Password == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "username, email, and password are required")
	}

	user, err := h.userSvc.CreateUser(c.Context(), service.CreateUserRequest{
		Username:  body.Username,
		Email:     body.Email,
		Password:  body.Password,
		ActorID:   actorID,
		IP:        getIP(c),
		UserAgent: c.Get("User-Agent"),
	})
	if err != nil {
		return h.mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"is_active":       user.IsActive,
		"must_change_pwd": user.MustChangePwd,
		"created_at":      user.CreatedAt,
	})
}

// GetUser maneja GET /admin/users/:id.
//
// Retorna el detalle completo de un usuario por su ID (UUID). Incluye campos
// sensibles como failed_attempts y locked_until que son útiles para diagnóstico
// y soporte. Si el ID no es un UUID válido retorna 400 sin consultar la base de datos.
//
// Códigos HTTP posibles:
//   - 200: detalle del usuario
//   - 400: el parámetro :id no es un UUID válido
//   - 401: JWT ausente o inválido
//   - 403: sin permiso admin.users.read
//   - 404: usuario no encontrado
//
// GetUser handles GET /admin/users/:id.
//
// @Summary     Obtener usuario
// @Description Retorna los detalles completos de un usuario por su ID.
// @Tags        Usuarios
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID del usuario (UUID)"
// @Success     200  {object}  SwaggerUserItem       "Detalles del usuario"
// @Failure     400  {object}  SwaggerErrorResponse  "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Failure     404  {object}  SwaggerErrorResponse  "Usuario no encontrado"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users/{id} [get]
func (h *AdminHandler) GetUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	user, err := h.userSvc.GetUser(c.Context(), id)
	if err != nil || user == nil {
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "user not found")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"is_active":       user.IsActive,
		"must_change_pwd": user.MustChangePwd,
		"last_login_at":   user.LastLoginAt,
		"failed_attempts": user.FailedAttempts,
		"locked_until":    user.LockedUntil,
		"created_at":      user.CreatedAt,
		"updated_at":      user.UpdatedAt,
	})
}

// UpdateUser maneja PUT /admin/users/:id.
//
// Actualiza los datos de un usuario de forma parcial: solo se modifican los campos
// que se incluyan en el body (username, email, is_active). Los campos ausentes
// conservan su valor actual. Esto se implementa con punteros opcionales (*string, *bool).
//
// Códigos HTTP posibles:
//   - 200: usuario actualizado
//   - 400: UUID inválido o body malformado
//   - 401: JWT ausente o inválido
//   - 403: sin permiso admin.users.write
//   - 404: usuario no encontrado
//   - 409: conflicto de unicidad (username o email duplicado)
//   - 500: error interno
//
// UpdateUser handles PUT /admin/users/:id.
//
// @Summary     Actualizar usuario
// @Description Actualiza los datos de un usuario. Solo se modifican los campos enviados.
// @Tags        Usuarios
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                   true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                   true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string                   true  "ID del usuario (UUID)"
// @Param       body           body     SwaggerUpdateUserRequest true  "Campos a actualizar"
// @Success     200  {object}  SwaggerUserItem       "Usuario actualizado"
// @Failure     400  {object}  SwaggerErrorResponse  "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Failure     404  {object}  SwaggerErrorResponse  "Usuario no encontrado"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users/{id} [put]
func (h *AdminHandler) UpdateUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Username *string `json:"username"`
		Email    *string `json:"email"`
		IsActive *bool   `json:"is_active"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	user, err := h.userSvc.UpdateUser(c.Context(), id, service.UpdateUserRequest{
		Username:  body.Username,
		Email:     body.Email,
		IsActive:  body.IsActive,
		ActorID:   actorID,
		IP:        getIP(c),
		UserAgent: c.Get("User-Agent"),
	})
	if err != nil {
		return h.mapServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"is_active":       user.IsActive,
		"must_change_pwd": user.MustChangePwd,
		"updated_at":      user.UpdatedAt,
	})
}

// UnlockUser maneja POST /admin/users/:id/unlock.
//
// Desbloquea una cuenta de usuario que fue bloqueada por superar el umbral de
// intentos fallidos de login. Restablece los contadores lockout_count, lockout_date
// y locked_until en la base de datos. Solo un administrador con el permiso
// correspondiente puede ejecutar esta acción.
//
// Códigos HTTP posibles:
//   - 204: usuario desbloqueado, sin cuerpo en la respuesta
//   - 400: UUID inválido
//   - 401: JWT ausente o inválido
//   - 403: sin permiso admin.users.write
//   - 500: error interno
//
// UnlockUser handles POST /admin/users/:id/unlock.
//
// @Summary     Desbloquear usuario
// @Description Desbloquea una cuenta de usuario que ha sido bloqueada por intentos fallidos de acceso.
// @Tags        Usuarios
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID del usuario (UUID)"
// @Success     204  "Usuario desbloqueado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse  "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users/{id}/unlock [post]
func (h *AdminHandler) UnlockUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.userSvc.UnlockUser(c.Context(), id, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return h.internalError(c, err, "admin: unlock user failed")
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ResetPassword maneja POST /admin/users/:id/reset-password.
//
// Genera una contraseña temporal aleatoria, la hashea con bcrypt y la guarda
// en la cuenta del usuario. Establece must_change_pwd=true para obligar al
// usuario a cambiarla en el próximo login. La contraseña temporal se devuelve
// en texto plano en la respuesta para que el administrador la comunique al usuario
// por un canal seguro (email, ticket, etc.).
//
// La contraseña temporal está exenta de la política de contraseñas para facilitar
// el proceso de restablecimiento.
//
// Códigos HTTP posibles:
//   - 200: contraseña temporal generada con campo "temporary_password"
//   - 400: UUID inválido
//   - 401: JWT ausente o inválido
//   - 403: sin permiso admin.users.write
//   - 500: error interno o fallo de generación de entropía
//
// ResetPassword handles POST /admin/users/:id/reset-password.
//
// @Summary     Restablecer contraseña
// @Description Genera una contraseña temporal para el usuario y lo obliga a cambiarla en el próximo inicio de sesión.
// @Tags        Usuarios
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID del usuario (UUID)"
// @Success     200  {object}  SwaggerResetPasswordResponse  "Contraseña temporal generada"
// @Failure     400  {object}  SwaggerErrorResponse          "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse          "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse          "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users/{id}/reset-password [post]
func (h *AdminHandler) ResetPassword(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	tempPwd, err := h.userSvc.ResetPassword(c.Context(), id, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return h.internalError(c, err, "admin: reset password failed")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"temporary_password": tempPwd,
	})
}

// AssignRole maneja POST /admin/users/:id/roles.
//
// Asigna un rol a un usuario con vigencia opcional (valid_from, valid_until).
// Si valid_until se omite, la asignación es permanente. El appID se extrae
// de los Locals del middleware AppKey para registrar la aplicación en la que
// se realizó la asignación (importante para la auditoría multi-tenant).
//
// Códigos HTTP posibles:
//   - 201: rol asignado con la información de la asignación
//   - 400: UUID inválido o body malformado
//   - 401: JWT ausente o inválido
//   - 403: sin permiso admin.users.write
//   - 409: el usuario ya tiene ese rol asignado
//   - 500: error interno
//
// AssignRole handles POST /admin/users/:id/roles.
//
// @Summary     Asignar rol a usuario
// @Description Asigna un rol a un usuario, opcionalmente con período de vigencia.
// @Tags        Usuarios
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                   true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                   true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string                   true  "ID del usuario (UUID)"
// @Param       body           body     SwaggerAssignRoleRequest true  "Datos de la asignación"
// @Success     201  {object}  SwaggerAssignedRoleResponse  "Rol asignado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse    "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse    "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse    "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users/{id}/roles [post]
func (h *AdminHandler) AssignRole(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	app := middleware.GetApp(c)
	appID := uuid.Nil
	if app != nil {
		appID = app.ID
	}

	var body struct {
		RoleID     uuid.UUID  `json:"role_id"`
		ValidFrom  *time.Time `json:"valid_from"`
		ValidUntil *time.Time `json:"valid_until"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.RoleID == uuid.Nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "role_id is required")
	}

	ur, err := h.userSvc.AssignRole(c.Context(), userID, service.AssignRoleRequest{
		RoleID:     body.RoleID,
		ValidFrom:  body.ValidFrom,
		ValidUntil: body.ValidUntil,
		ActorID:    actorID,
		AppID:      appID,
		IP:         getIP(c),
		UserAgent:  c.Get("User-Agent"),
	})
	if err != nil {
		return h.mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":          ur.ID,
		"user_id":     userID,
		"role_id":     ur.RoleID,
		"role_name":   ur.RoleName,
		"valid_from":  ur.ValidFrom,
		"valid_until": ur.ValidUntil,
		"granted_by":  ur.GrantedBy,
	})
}

// RevokeRole maneja DELETE /admin/users/:id/roles/:rid.
//
// Elimina una asignación de rol específica de un usuario. El parámetro :rid
// es el ID del registro en la tabla user_roles (no el ID del rol en sí),
// lo que permite revocar una asignación temporal sin afectar otras asignaciones
// del mismo rol al mismo usuario con distintas vigencias.
//
// Códigos HTTP posibles:
//   - 204: asignación revocada, sin cuerpo en la respuesta
//   - 400: UUID inválido en :id o :rid
//   - 401: JWT ausente o inválido
//   - 403: sin permiso admin.users.write
//   - 500: error interno
//
// RevokeRole handles DELETE /admin/users/:id/roles/:rid.
//
// @Summary     Revocar rol de usuario
// @Description Elimina la asignación de un rol a un usuario.
// @Tags        Usuarios
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID del usuario (UUID)"
// @Param       rid            path     string  true  "ID de la asignación de rol (UUID)"
// @Success     204  "Rol revocado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse  "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users/{id}/roles/{rid} [delete]
func (h *AdminHandler) RevokeRole(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}
	rid, err := uuid.Parse(c.Params("rid"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role assignment id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.userSvc.RevokeRole(c.Context(), userID, rid, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return h.internalError(c, err, "admin: revoke role failed")
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// AssignPermission maneja POST /admin/users/:id/permissions.
//
// Asigna un permiso directo a un usuario (sin pasar por un rol). Esto permite
// excepciones puntuales en la gestión de accesos. Al igual que AssignRole,
// soporta vigencia opcional y registra el appID para la auditoría.
//
// Códigos HTTP posibles:
//   - 201: permiso asignado con la información de la asignación
//   - 400: UUID inválido o body malformado
//   - 401: JWT ausente o inválido
//   - 403: sin permiso admin.users.write
//   - 409: el usuario ya tiene ese permiso directo asignado
//   - 500: error interno
//
// AssignPermission handles POST /admin/users/:id/permissions.
//
// @Summary     Asignar permiso a usuario
// @Description Asigna un permiso directo a un usuario, opcionalmente con período de vigencia.
// @Tags        Usuarios
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                         true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                         true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string                         true  "ID del usuario (UUID)"
// @Param       body           body     SwaggerAssignPermissionRequest true  "Datos de la asignación"
// @Success     201  {object}  SwaggerAssignedPermissionResponse  "Permiso asignado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse    "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse    "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse    "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users/{id}/permissions [post]
func (h *AdminHandler) AssignPermission(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	app := middleware.GetApp(c)
	appID := uuid.Nil
	if app != nil {
		appID = app.ID
	}

	var body struct {
		PermissionID uuid.UUID  `json:"permission_id"`
		ValidFrom    *time.Time `json:"valid_from"`
		ValidUntil   *time.Time `json:"valid_until"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	up, err := h.userSvc.AssignPermission(c.Context(), userID, service.AssignPermissionRequest{
		PermissionID: body.PermissionID,
		ValidFrom:    body.ValidFrom,
		ValidUntil:   body.ValidUntil,
		ActorID:      actorID,
		AppID:        appID,
		IP:           getIP(c),
		UserAgent:    c.Get("User-Agent"),
	})
	if err != nil {
		return h.mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":            up.ID,
		"user_id":       userID,
		"permission_id": up.PermissionID,
		"valid_from":    up.ValidFrom,
		"valid_until":   up.ValidUntil,
		"granted_by":    up.GrantedBy,
	})
}

// RevokePermission maneja DELETE /admin/users/:id/permissions/:pid.
//
// Elimina una asignación directa de permiso a un usuario. El parámetro :pid
// es el ID del registro en user_permissions, no del permiso en sí.
//
// Códigos HTTP posibles:
//   - 204: asignación revocada, sin cuerpo en la respuesta
//   - 400: UUID inválido en :id o :pid
//   - 401: JWT ausente o inválido
//   - 403: sin permiso admin.users.write
//   - 500: error interno
//
// RevokePermission handles DELETE /admin/users/:id/permissions/:pid.
//
// @Summary     Revocar permiso de usuario
// @Description Elimina la asignación directa de un permiso a un usuario.
// @Tags        Usuarios
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID del usuario (UUID)"
// @Param       pid            path     string  true  "ID de la asignación de permiso (UUID)"
// @Success     204  "Permiso revocado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse  "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users/{id}/permissions/{pid} [delete]
func (h *AdminHandler) RevokePermission(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}
	pid, err := uuid.Parse(c.Params("pid"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid permission assignment id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.userSvc.RevokePermission(c.Context(), userID, pid, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return h.internalError(c, err, "admin: revoke permission failed")
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// AssignCostCenters maneja POST /admin/users/:id/cost-centers.
//
// Asigna uno o más centros de costo a un usuario en una sola operación,
// reemplazando las asignaciones previas (semántica de set completo, no append).
// Si cost_center_ids está vacío, elimina todas las asignaciones del usuario.
// Esto simplifica la sincronización desde sistemas externos (ERP, RRHH).
//
// Códigos HTTP posibles:
//   - 201: asignaciones realizadas, responde con el conteo {"assigned": N}
//   - 400: UUID inválido o body malformado
//   - 401: JWT ausente o inválido
//   - 403: sin permiso admin.users.write
//   - 500: error interno
//
// AssignCostCenters handles POST /admin/users/:id/cost-centers.
//
// @Summary     Asignar centros de costo a usuario
// @Description Asigna uno o más centros de costo a un usuario, reemplazando las asignaciones previas.
// @Tags        Usuarios
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                            true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                            true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string                            true  "ID del usuario (UUID)"
// @Param       body           body     SwaggerAssignCostCentersRequest   true  "Centros de costo a asignar"
// @Success     201  {object}  SwaggerAssignedCountResponse  "Centros de costo asignados"
// @Failure     400  {object}  SwaggerErrorResponse          "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse          "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse          "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/users/{id}/cost-centers [post]
func (h *AdminHandler) AssignCostCenters(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid user id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	app := middleware.GetApp(c)
	appID := uuid.Nil
	if app != nil {
		appID = app.ID
	}

	var body struct {
		CostCenterIDs []uuid.UUID `json:"cost_center_ids"`
		ValidFrom     *time.Time  `json:"valid_from"`
		ValidUntil    *time.Time  `json:"valid_until"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	if err := h.userSvc.AssignCostCenters(c.Context(), userID, service.AssignCostCentersRequest{
		CostCenterIDs: body.CostCenterIDs,
		ValidFrom:     body.ValidFrom,
		ValidUntil:    body.ValidUntil,
		ActorID:       actorID,
		AppID:         appID,
		IP:            getIP(c),
		UserAgent:     c.Get("User-Agent"),
	}); err != nil {
		return h.mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"assigned": len(body.CostCenterIDs)})
}

// ---- ROLE ENDPOINTS ----

// ListRoles maneja GET /admin/roles.
//
// Retorna la lista paginada de roles. Si el middleware AppKey inyectó una
// aplicación en Locals, filtra los roles por esa aplicación. Esto permite
// que cada aplicación cliente vea solo sus propios roles.
//
// Códigos HTTP posibles:
//   - 200: lista paginada de roles (puede ser lista vacía)
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.roles.read
//   - 500: error de base de datos
//
// ListRoles handles GET /admin/roles.
//
// @Summary     Listar roles
// @Description Retorna la lista paginada de roles de la aplicación.
// @Tags        Roles
// @Produce     json
// @Param       X-App-Key      header   string  true   "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true   "Token JWT. Formato: Bearer {token}"
// @Param       page           query    int     false  "Número de página (default: 1)"
// @Param       page_size      query    int     false  "Elementos por página (default: 20, max: 100)"
// @Success     200  {object}  SwaggerPaginatedRoles  "Lista paginada de roles"
// @Failure     401  {object}  SwaggerErrorResponse   "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse   "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/roles [get]
func (h *AdminHandler) ListRoles(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)
	app := middleware.GetApp(c)

	filter := postgres.RoleFilter{Page: page, PageSize: pageSize}
	if app != nil {
		appID := app.ID
		filter.ApplicationID = &appID
	}

	roles, total, err := h.roleSvc.ListRoles(c.Context(), filter)
	if err != nil {
		return h.internalError(c, err, "admin: list roles failed")
	}

	data := make([]fiber.Map, 0, len(roles))
	for _, r := range roles {
		data = append(data, fiber.Map{
			"id":          r.ID,
			"name":        r.Name,
			"description": r.Description,
			"is_system":   r.IsSystem,
			"is_active":   r.IsActive,
			"created_at":  r.CreatedAt,
		})
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(data, page, pageSize, total))
}

// CreateRole maneja POST /admin/roles.
//
// Crea un nuevo rol dentro de la aplicación identificada por X-App-Key.
// Si la aplicación no está en Locals (falla del middleware AppKey) rechaza
// la operación con 401. El campo description es opcional.
//
// Códigos HTTP posibles:
//   - 201: rol creado exitosamente
//   - 400: body inválido o campo "name" vacío
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.roles.write
//   - 409: ya existe un rol con ese nombre en la aplicación
//   - 500: error interno
//
// CreateRole handles POST /admin/roles.
//
// @Summary     Crear rol
// @Description Crea un nuevo rol en la aplicación actual.
// @Tags        Roles
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                   true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                   true  "Token JWT. Formato: Bearer {token}"
// @Param       body           body     SwaggerCreateRoleRequest true  "Datos del nuevo rol"
// @Success     201  {object}  SwaggerRoleItem       "Rol creado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse  "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/roles [post]
func (h *AdminHandler) CreateRole(c *fiber.Ctx) error {
	app := middleware.GetApp(c)
	if app == nil {
		return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid application")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Name == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "name is required")
	}

	role, err := h.roleSvc.CreateRole(c.Context(), app.ID, body.Name, body.Description, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return h.mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(role)
}

// GetRole maneja GET /admin/roles/:id.
//
// Retorna el detalle de un rol incluyendo la lista de permisos asignados y el
// número de usuarios que tienen ese rol activo. Útil para mostrar la vista de
// detalle en el panel de administración antes de editar o eliminar el rol.
//
// Códigos HTTP posibles:
//   - 200: detalle del rol con permisos y users_count
//   - 400: UUID inválido
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.roles.read
//   - 404: rol no encontrado
//
// GetRole handles GET /admin/roles/:id.
//
// @Summary     Obtener rol
// @Description Retorna los detalles de un rol, incluyendo sus permisos y la cantidad de usuarios asignados.
// @Tags        Roles
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID del rol (UUID)"
// @Success     200  {object}  SwaggerRoleDetailResponse  "Detalles del rol"
// @Failure     400  {object}  SwaggerErrorResponse    "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse    "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse    "Sin permiso"
// @Failure     404  {object}  SwaggerErrorResponse    "Rol no encontrado"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/roles/{id} [get]
func (h *AdminHandler) GetRole(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role id")
	}

	role, err := h.roleSvc.GetRole(c.Context(), id)
	if err != nil {
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "role not found")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":          role.ID,
		"name":        role.Name,
		"description": role.Description,
		"is_system":   role.IsSystem,
		"is_active":   role.IsActive,
		"permissions": role.Permissions,
		"users_count": role.UsersCount,
		"created_at":  role.CreatedAt,
		"updated_at":  role.UpdatedAt,
	})
}

// UpdateRole maneja PUT /admin/roles/:id.
//
// Actualiza el nombre y/o descripción de un rol. Los roles de sistema (is_system=true)
// pueden recibir actualizaciones de descripción pero no de nombre, según la
// lógica implementada en RoleService.
//
// Códigos HTTP posibles:
//   - 200: rol actualizado
//   - 400: UUID inválido o body malformado
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.roles.write
//   - 404: rol no encontrado
//   - 500: error interno
//
// UpdateRole handles PUT /admin/roles/:id.
//
// @Summary     Actualizar rol
// @Description Actualiza el nombre y/o descripción de un rol existente.
// @Tags        Roles
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                   true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                   true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string                   true  "ID del rol (UUID)"
// @Param       body           body     SwaggerUpdateRoleRequest true  "Campos a actualizar"
// @Success     200  {object}  SwaggerRoleItem       "Rol actualizado"
// @Failure     400  {object}  SwaggerErrorResponse  "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Failure     404  {object}  SwaggerErrorResponse  "Rol no encontrado"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/roles/{id} [put]
func (h *AdminHandler) UpdateRole(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	role, err := h.roleSvc.UpdateRole(c.Context(), id, body.Name, body.Description, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return h.mapServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(role)
}

// DeleteRole maneja DELETE /admin/roles/:id.
//
// Desactiva un rol estableciendo is_active=false (borrado lógico). Los roles
// de sistema (is_system=true) no pueden eliminarse. Los usuarios que tenían
// ese rol dejan de obtener los permisos asociados en su próxima verificación
// de autorización, ya que el caché de permisos se invalida.
//
// Códigos HTTP posibles:
//   - 204: rol desactivado, sin cuerpo en la respuesta
//   - 400: UUID inválido
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.roles.write o intento de eliminar rol de sistema
//   - 500: error interno
//
// DeleteRole handles DELETE /admin/roles/:id.
//
// @Summary     Eliminar rol
// @Description Desactiva un rol (no se elimina físicamente). Los usuarios con este rol pierden los permisos asociados.
// @Tags        Roles
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID del rol (UUID)"
// @Success     204  "Rol eliminado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse  "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/roles/{id} [delete]
func (h *AdminHandler) DeleteRole(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.roleSvc.DeactivateRole(c.Context(), id, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return h.mapServiceError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// AddRolePermission maneja POST /admin/roles/:id/permissions.
//
// Asigna uno o más permisos a un rol en una sola petición. Itera sobre
// permission_ids y llama al servicio por cada uno. Si alguna asignación falla
// (ej. permiso ya asignado o no encontrado), la operación se detiene y retorna
// el error correspondiente; las asignaciones previas del mismo batch no se revierten.
//
// Códigos HTTP posibles:
//   - 201: permisos asignados con el conteo {"assigned": N}
//   - 400: UUID de rol inválido o body malformado
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.roles.write
//   - 409: algún permiso ya estaba asignado al rol
//   - 500: error interno
//
// AddRolePermission handles POST /admin/roles/:id/permissions.
//
// @Summary     Agregar permisos a rol
// @Description Asigna uno o más permisos a un rol existente.
// @Tags        Roles
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                        true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                        true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string                        true  "ID del rol (UUID)"
// @Param       body           body     SwaggerAddRolePermissionRequest true "Permisos a asignar"
// @Success     201  {object}  SwaggerAssignedCountResponse  "Permisos asignados al rol"
// @Failure     400  {object}  SwaggerErrorResponse          "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse          "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse          "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/roles/{id}/permissions [post]
func (h *AdminHandler) AddRolePermission(c *fiber.Ctx) error {
	roleID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		PermissionIDs []uuid.UUID `json:"permission_ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	for _, pid := range body.PermissionIDs {
		if err := h.roleSvc.AddPermissionToRole(c.Context(), roleID, pid, actorID, getIP(c), c.Get("User-Agent")); err != nil {
			return h.mapServiceError(c, err)
		}
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"assigned": len(body.PermissionIDs)})
}

// RemoveRolePermission maneja DELETE /admin/roles/:id/permissions/:pid.
//
// Elimina la relación entre un rol y un permiso. El parámetro :pid es el ID
// del permiso (no del registro de la tabla intermedia). Esto afecta a todos
// los usuarios que tengan ese rol, quienes pierden el permiso en su próxima
// verificación de autorización.
//
// Códigos HTTP posibles:
//   - 204: permiso removido del rol, sin cuerpo en la respuesta
//   - 400: UUID inválido en :id o :pid
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.roles.write
//   - 500: error interno
//
// RemoveRolePermission handles DELETE /admin/roles/:id/permissions/:pid.
//
// @Summary     Remover permiso de rol
// @Description Elimina un permiso de un rol existente.
// @Tags        Roles
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID del rol (UUID)"
// @Param       pid            path     string  true  "ID del permiso (UUID)"
// @Success     204  "Permiso removido del rol exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse  "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/roles/{id}/permissions/{pid} [delete]
func (h *AdminHandler) RemoveRolePermission(c *fiber.Ctx) error {
	roleID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid role id")
	}
	pid, err := uuid.Parse(c.Params("pid"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid permission id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.roleSvc.RemovePermissionFromRole(c.Context(), roleID, pid, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return h.mapServiceError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ---- PERMISSION ENDPOINTS ----

// ListPermissions maneja GET /admin/permissions.
//
// Retorna la lista paginada de permisos. Si el middleware AppKey inyectó una
// aplicación, filtra los permisos de esa aplicación. Los permisos siguen el
// formato de código punteado: "<app>.<modulo>.<accion>" (ej: "erp.reportes.read").
//
// Códigos HTTP posibles:
//   - 200: lista paginada de permisos (puede ser lista vacía)
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.permissions.read
//   - 500: error de base de datos
//
// ListPermissions handles GET /admin/permissions.
//
// @Summary     Listar permisos
// @Description Retorna la lista paginada de permisos de la aplicación.
// @Tags        Permisos
// @Produce     json
// @Param       X-App-Key      header   string  true   "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true   "Token JWT. Formato: Bearer {token}"
// @Param       page           query    int     false  "Número de página (default: 1)"
// @Param       page_size      query    int     false  "Elementos por página (default: 20, max: 100)"
// @Success     200  {object}  SwaggerPaginatedPermissions  "Lista paginada de permisos"
// @Failure     401  {object}  SwaggerErrorResponse         "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse         "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/permissions [get]
func (h *AdminHandler) ListPermissions(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)
	app := middleware.GetApp(c)

	filter := postgres.PermissionFilter{Page: page, PageSize: pageSize}
	if app != nil {
		appID := app.ID
		filter.ApplicationID = &appID
	}

	perms, total, err := h.permSvc.ListPermissions(c.Context(), filter)
	if err != nil {
		return h.internalError(c, err, "admin: list permissions failed")
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(perms, page, pageSize, total))
}

// CreatePermission maneja POST /admin/permissions.
//
// Crea un nuevo permiso dentro de la aplicación identificada por X-App-Key.
// El campo scope_type determina el alcance del permiso y acepta los valores:
// "global", "module", "resource" o "action". El código del permiso debe ser
// único dentro de la aplicación.
//
// Códigos HTTP posibles:
//   - 201: permiso creado exitosamente
//   - 400: body inválido, campos code o scope_type vacíos
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.permissions.write
//   - 409: ya existe un permiso con ese código en la aplicación
//   - 500: error interno
//
// CreatePermission handles POST /admin/permissions.
//
// @Summary     Crear permiso
// @Description Crea un nuevo permiso en la aplicación actual.
// @Tags        Permisos
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                         true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                         true  "Token JWT. Formato: Bearer {token}"
// @Param       body           body     SwaggerCreatePermissionRequest true  "Datos del nuevo permiso"
// @Success     201  {object}  SwaggerPermissionItem  "Permiso creado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse   "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse   "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse   "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/permissions [post]
func (h *AdminHandler) CreatePermission(c *fiber.Ctx) error {
	app := middleware.GetApp(c)
	if app == nil {
		return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid application")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Code        string `json:"code"`
		Description string `json:"description"`
		ScopeType   string `json:"scope_type"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Code == "" || body.ScopeType == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "code and scope_type are required")
	}

	perm, err := h.permSvc.CreatePermission(c.Context(), app.ID, body.Code, body.Description, body.ScopeType, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return h.mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(perm)
}

// DeletePermission maneja DELETE /admin/permissions/:id.
//
// Elimina físicamente un permiso del sistema. La operación falla (retorna error)
// si el permiso está asignado a algún rol o usuario, para evitar huérfanos en
// la tabla de autorización. El servicio verifica estas dependencias antes de
// eliminar.
//
// Códigos HTTP posibles:
//   - 204: permiso eliminado, sin cuerpo en la respuesta
//   - 400: UUID inválido
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.permissions.write
//   - 409: el permiso está en uso (asignado a roles o usuarios)
//   - 500: error interno
//
// DeletePermission handles DELETE /admin/permissions/:id.
//
// @Summary     Eliminar permiso
// @Description Elimina un permiso del sistema. Falla si el permiso está asignado a roles o usuarios.
// @Tags        Permisos
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID del permiso (UUID)"
// @Success     204  "Permiso eliminado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse  "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse  "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse  "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/permissions/{id} [delete]
func (h *AdminHandler) DeletePermission(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid permission id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	if err := h.permSvc.DeletePermission(c.Context(), id, actorID, getIP(c), c.Get("User-Agent")); err != nil {
		return h.mapServiceError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ---- COST CENTER ENDPOINTS ----

// ListCostCenters maneja GET /admin/cost-centers.
//
// Retorna la lista paginada de centros de costo. Si el middleware AppKey inyectó
// una aplicación, filtra los centros de costo de esa aplicación. Los centros de
// costo son unidades organizacionales usadas para restringir el alcance de
// acciones de los usuarios (ej. un supervisor solo puede operar en su ceco).
//
// Códigos HTTP posibles:
//   - 200: lista paginada de centros de costo
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.cost_centers.read
//   - 500: error de base de datos
//
// ListCostCenters handles GET /admin/cost-centers.
//
// @Summary     Listar centros de costo
// @Description Retorna la lista paginada de centros de costo de la aplicación.
// @Tags        Centros de Costo
// @Produce     json
// @Param       X-App-Key      header   string  true   "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true   "Token JWT. Formato: Bearer {token}"
// @Param       page           query    int     false  "Número de página (default: 1)"
// @Param       page_size      query    int     false  "Elementos por página (default: 20, max: 100)"
// @Success     200  {object}  SwaggerPaginatedCostCenters  "Lista paginada de centros de costo"
// @Failure     401  {object}  SwaggerErrorResponse         "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse         "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/cost-centers [get]
func (h *AdminHandler) ListCostCenters(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)
	app := middleware.GetApp(c)

	filter := postgres.CCFilter{Page: page, PageSize: pageSize}
	if app != nil {
		appID := app.ID
		filter.ApplicationID = &appID
	}

	ccs, total, err := h.ccSvc.ListCostCenters(c.Context(), filter)
	if err != nil {
		return h.internalError(c, err, "admin: list cost centers failed")
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(ccs, page, pageSize, total))
}

// CreateCostCenter maneja POST /admin/cost-centers.
//
// Crea un nuevo centro de costo en la aplicación identificada por X-App-Key.
// Los campos code y name son obligatorios. El code debe ser único dentro de
// la aplicación y sirve como identificador legible (ej. "CC-001", "RRHH").
//
// Códigos HTTP posibles:
//   - 201: centro de costo creado exitosamente
//   - 400: body inválido o campos code/name vacíos
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.cost_centers.write
//   - 409: ya existe un centro de costo con ese código
//   - 500: error interno
//
// CreateCostCenter handles POST /admin/cost-centers.
//
// @Summary     Crear centro de costo
// @Description Crea un nuevo centro de costo en la aplicación actual.
// @Tags        Centros de Costo
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                          true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                          true  "Token JWT. Formato: Bearer {token}"
// @Param       body           body     SwaggerCreateCostCenterRequest  true  "Datos del nuevo centro de costo"
// @Success     201  {object}  SwaggerCostCenterItem  "Centro de costo creado exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse   "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse   "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse   "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/cost-centers [post]
func (h *AdminHandler) CreateCostCenter(c *fiber.Ctx) error {
	app := middleware.GetApp(c)
	if app == nil {
		return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid application")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Code == "" || body.Name == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "code and name are required")
	}

	cc, err := h.ccSvc.CreateCostCenter(c.Context(), app.ID, body.Code, body.Name, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return h.mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(cc)
}

// UpdateCostCenter maneja PUT /admin/cost-centers/:id.
//
// Actualiza el nombre y/o estado activo de un centro de costo. El valor por
// defecto de is_active es true para evitar que una omisión accidental desactive
// el ceco. El código del centro de costo no puede modificarse después de la creación.
//
// Códigos HTTP posibles:
//   - 200: centro de costo actualizado
//   - 400: UUID inválido o body malformado
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.cost_centers.write
//   - 404: centro de costo no encontrado
//   - 500: error interno
//
// UpdateCostCenter handles PUT /admin/cost-centers/:id.
//
// @Summary     Actualizar centro de costo
// @Description Actualiza el nombre y/o estado de un centro de costo existente.
// @Tags        Centros de Costo
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                          true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                          true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string                          true  "ID del centro de costo (UUID)"
// @Param       body           body     SwaggerUpdateCostCenterRequest  true  "Campos a actualizar"
// @Success     200  {object}  SwaggerCostCenterItem  "Centro de costo actualizado"
// @Failure     400  {object}  SwaggerErrorResponse   "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse   "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse   "Sin permiso"
// @Failure     404  {object}  SwaggerErrorResponse   "Centro de costo no encontrado"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/cost-centers/{id} [put]
func (h *AdminHandler) UpdateCostCenter(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid cost center id")
	}

	claims := middleware.GetClaims(c)
	actorID := uuid.Nil
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	var body struct {
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	}
	body.IsActive = true // default
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}

	cc, err := h.ccSvc.UpdateCostCenter(c.Context(), id, body.Name, body.IsActive, actorID, getIP(c), c.Get("User-Agent"))
	if err != nil {
		return h.mapServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(cc)
}

// ---- AUDIT LOGS ENDPOINT ----

// ListAuditLogs maneja GET /admin/audit-logs.
//
// Retorna la lista paginada de eventos de auditoría con soporte para múltiples
// filtros combinables: tipo de evento, usuario afectado, actor que ejecutó la
// acción, aplicación, rango de fechas y resultado (éxito/fallo).
//
// Los filtros UUID (user_id, actor_id, application_id) se ignoran silenciosamente
// si no son UUIDs válidos para evitar errores de validación innecesarios.
// Las fechas deben estar en formato RFC3339 (ej. 2025-01-01T00:00:00Z).
//
// Los logs de auditoría son inmutables: solo se pueden leer, nunca modificar.
//
// Códigos HTTP posibles:
//   - 200: lista paginada de eventos de auditoría
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.audit.read
//   - 500: error de base de datos
//
// ListAuditLogs handles GET /admin/audit-logs.
//
// @Summary     Listar registros de auditoría
// @Description Retorna la lista paginada de eventos de auditoría. Permite filtrar por tipo de evento, usuario, actor, aplicación, fechas y resultado.
// @Tags        Auditoría
// @Produce     json
// @Param       X-App-Key      header   string  true   "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true   "Token JWT. Formato: Bearer {token}"
// @Param       page           query    int     false  "Número de página (default: 1)"
// @Param       page_size      query    int     false  "Elementos por página (default: 20, max: 100)"
// @Param       event_type     query    string  false  "Filtrar por tipo de evento (ej: LOGIN_SUCCESS)"
// @Param       user_id        query    string  false  "Filtrar por ID de usuario (UUID)"
// @Param       actor_id       query    string  false  "Filtrar por ID del actor (UUID)"
// @Param       application_id query    string  false  "Filtrar por ID de aplicación (UUID)"
// @Param       from_date      query    string  false  "Fecha inicio (RFC3339, ej: 2025-01-01T00:00:00Z)"
// @Param       to_date        query    string  false  "Fecha fin (RFC3339, ej: 2025-12-31T23:59:59Z)"
// @Param       success        query    bool    false  "Filtrar por resultado (true=exitoso, false=fallido)"
// @Success     200  {object}  SwaggerPaginatedAuditLogs  "Lista paginada de registros de auditoría"
// @Failure     401  {object}  SwaggerErrorResponse       "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse       "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/audit-logs [get]
func (h *AdminHandler) ListAuditLogs(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)

	filter := postgres.AuditFilter{
		Page:      page,
		PageSize:  pageSize,
		EventType: c.Query("event_type", ""),
	}

	if v := c.Query("user_id", ""); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.UserID = &id
		}
	}
	if v := c.Query("actor_id", ""); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.ActorID = &id
		}
	}
	if v := c.Query("application_id", ""); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.ApplicationID = &id
		}
	}
	if v := c.Query("from_date", ""); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.FromDate = &t
		}
	}
	if v := c.Query("to_date", ""); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.ToDate = &t
		}
	}
	if v := c.Query("success", ""); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			filter.Success = &b
		}
	}

	logs, total, err := h.auditRepo.List(c.Context(), filter)
	if err != nil {
		return h.internalError(c, err, "admin: list audit logs failed")
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(logs, page, pageSize, total))
}

// ---- APPLICATION ENDPOINTS ----

// ListApplications maneja GET /admin/applications.
//
// Retorna la lista paginada de aplicaciones cliente registradas en Sentinel.
// Soporta filtrado por búsqueda de texto (nombre o slug) y por estado activo.
// El campo is_system se calcula en memoria: es true si el slug es "system".
//
// Códigos HTTP posibles:
//   - 200: lista paginada de aplicaciones
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.applications.read
//   - 500: error de base de datos
//
// ListApplications handles GET /admin/applications.
//
// @Summary     Listar aplicaciones
// @Description Retorna la lista paginada de aplicaciones cliente registradas en el sistema.
// @Tags        Aplicaciones
// @Produce     json
// @Param       X-App-Key      header   string  true   "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true   "Token JWT. Formato: Bearer {token}"
// @Param       page           query    int     false  "Número de página (default: 1)"
// @Param       page_size      query    int     false  "Elementos por página (default: 20, max: 100)"
// @Param       search         query    string  false  "Búsqueda por nombre o slug"
// @Param       is_active      query    bool    false  "Filtrar por estado activo"
// @Success     200  {object}  SwaggerPaginatedApplications  "Lista paginada de aplicaciones"
// @Failure     401  {object}  SwaggerErrorResponse          "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse          "Sin permiso"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/applications [get]
func (h *AdminHandler) ListApplications(c *fiber.Ctx) error {
	page, pageSize := parsePagination(c)
	search := c.Query("search", "")

	var isActive *bool
	if v := c.Query("is_active", ""); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			isActive = &b
		}
	}

	apps, total, err := h.appRepo.List(c.Context(), postgres.ApplicationFilter{
		Search:   search,
		IsActive: isActive,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return h.internalError(c, err, "admin: list applications failed")
	}

	data := make([]fiber.Map, 0, len(apps))
	for _, a := range apps {
		data = append(data, fiber.Map{
			"id":         a.ID,
			"name":       a.Name,
			"slug":       a.Slug,
			"is_active":  a.IsActive,
			"is_system":  a.Slug == "system",
			"created_at": a.CreatedAt,
			"updated_at": a.UpdatedAt,
		})
	}

	return c.Status(fiber.StatusOK).JSON(paginatedResponse(data, page, pageSize, total))
}

// GetApplication maneja GET /admin/applications/:id.
//
// Retorna el detalle de una aplicación incluyendo su clave secreta (secret_key).
// La clave secreta solo se expone en este endpoint y en CreateApplication/RotateApplicationKey,
// nunca en listas. Solo administradores con el permiso correspondiente pueden acceder.
//
// Códigos HTTP posibles:
//   - 200: detalle de la aplicación con secret_key
//   - 400: UUID inválido
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.applications.read
//   - 404: aplicación no encontrada
//
// GetApplication handles GET /admin/applications/:id.
//
// @Summary     Obtener aplicación
// @Description Retorna los detalles de una aplicación, incluyendo su clave secreta.
// @Tags        Aplicaciones
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID de la aplicación (UUID)"
// @Success     200  {object}  SwaggerApplicationItem  "Detalles de la aplicación"
// @Failure     400  {object}  SwaggerErrorResponse    "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse    "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse    "Sin permiso"
// @Failure     404  {object}  SwaggerErrorResponse    "Aplicación no encontrada"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/applications/{id} [get]
func (h *AdminHandler) GetApplication(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid application id")
	}

	app, err := h.appRepo.FindByID(c.Context(), id)
	if err != nil || app == nil {
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "application not found")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":         app.ID,
		"name":       app.Name,
		"slug":       app.Slug,
		"secret_key": app.SecretKey,
		"is_active":  app.IsActive,
		"is_system":  app.Slug == "system",
		"created_at": app.CreatedAt,
		"updated_at": app.UpdatedAt,
	})
}

// CreateApplication maneja POST /admin/applications.
//
// Registra una nueva aplicación cliente en Sentinel. Genera automáticamente
// una clave secreta aleatoria de 32 bytes (Base64 URL-safe sin padding) usando
// crypto/rand para asegurar entropía criptográfica. El slug debe seguir el
// formato kebab-case validado por slugRegexp.
//
// La clave secreta se devuelve en el body de esta respuesta y es la única
// oportunidad de verla en texto plano; si se pierde se debe rotar con
// POST /admin/applications/:id/rotate-key.
//
// Maneja el error de unicidad de PostgreSQL (código 23505) para responder
// 409 en lugar de 500 cuando el nombre ya existe.
//
// Códigos HTTP posibles:
//   - 201: aplicación creada con secret_key en texto plano
//   - 400: body inválido, campos faltantes o slug con formato incorrecto
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.applications.write
//   - 409: ya existe una aplicación con ese slug o nombre
//   - 500: error interno o fallo de generación de entropía
//
// CreateApplication handles POST /admin/applications.
//
// @Summary     Crear aplicación
// @Description Registra una nueva aplicación cliente en el sistema y genera su clave secreta.
// @Tags        Aplicaciones
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                           true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                           true  "Token JWT. Formato: Bearer {token}"
// @Param       body           body     SwaggerCreateApplicationRequest  true  "Datos de la nueva aplicación"
// @Success     201  {object}  SwaggerApplicationItem  "Aplicación creada exitosamente"
// @Failure     400  {object}  SwaggerErrorResponse    "Datos inválidos o slug con formato incorrecto"
// @Failure     401  {object}  SwaggerErrorResponse    "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse    "Sin permiso"
// @Failure     409  {object}  SwaggerErrorResponse    "Ya existe una aplicación con este slug"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/applications [post]
func (h *AdminHandler) CreateApplication(c *fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Name == "" || body.Slug == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "name and slug are required")
	}
	if !slugRegexp.MatchString(body.Slug) {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "slug must be lowercase alphanumeric with hyphens (e.g. my-app)")
	}

	existing, err := h.appRepo.FindBySlug(c.Context(), body.Slug)
	if err != nil {
		return h.internalError(c, err, "admin: find application by slug failed")
	}
	if existing != nil {
		return respondError(c, fiber.StatusConflict, "CONFLICT", "an application with this slug already exists")
	}

	secretKey, err := generateAppSecretKey()
	if err != nil {
		return h.internalError(c, err, "admin: generate secret key failed")
	}

	app := &domain.Application{
		ID:        uuid.New(),
		Name:      body.Name,
		Slug:      body.Slug,
		SecretKey: secretKey,
		IsActive:  true,
	}
	if err := h.appRepo.Create(c.Context(), app); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return respondError(c, fiber.StatusConflict, "CONFLICT", "an application with this name already exists")
		}
		return h.internalError(c, err, "admin: create application failed")
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":         app.ID,
		"name":       app.Name,
		"slug":       app.Slug,
		"secret_key": app.SecretKey,
		"is_active":  app.IsActive,
		"is_system":  false,
		"created_at": app.CreatedAt,
	})
}

// UpdateApplication maneja PUT /admin/applications/:id.
//
// Actualiza el nombre y/o estado activo de una aplicación. Protege la aplicación
// del sistema (slug="system") de modificaciones: si se intenta actualizar retorna 403.
// Los campos omitidos conservan sus valores actuales (partial update con defaults
// cargados desde la base de datos antes de aplicar el body).
//
// Códigos HTTP posibles:
//   - 200: aplicación actualizada
//   - 400: UUID inválido o body malformado
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.applications.write o intento de modificar app del sistema
//   - 404: aplicación no encontrada
//   - 500: error interno
//
// UpdateApplication handles PUT /admin/applications/:id.
//
// @Summary     Actualizar aplicación
// @Description Actualiza el nombre y/o estado de una aplicación. La aplicación del sistema no puede ser modificada.
// @Tags        Aplicaciones
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                           true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                           true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string                           true  "ID de la aplicación (UUID)"
// @Param       body           body     SwaggerUpdateApplicationRequest  true  "Campos a actualizar"
// @Success     200  {object}  SwaggerApplicationItem  "Aplicación actualizada"
// @Failure     400  {object}  SwaggerErrorResponse    "Datos inválidos"
// @Failure     401  {object}  SwaggerErrorResponse    "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse    "Sin permiso o aplicación de sistema"
// @Failure     404  {object}  SwaggerErrorResponse    "Aplicación no encontrada"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/applications/{id} [put]
func (h *AdminHandler) UpdateApplication(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid application id")
	}

	existing, err := h.appRepo.FindByID(c.Context(), id)
	if err != nil || existing == nil {
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "application not found")
	}
	if existing.Slug == "system" {
		return respondError(c, fiber.StatusForbidden, "FORBIDDEN", "the system application cannot be modified")
	}

	var body struct {
		Name     string `json:"name"`
		IsActive *bool  `json:"is_active"`
	}
	body.IsActive = &existing.IsActive
	if err := c.BodyParser(&body); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if body.Name == "" {
		body.Name = existing.Name
	}

	isActive := existing.IsActive
	if body.IsActive != nil {
		isActive = *body.IsActive
	}

	updated, err := h.appRepo.Update(c.Context(), id, body.Name, isActive)
	if err != nil || updated == nil {
		if err == nil {
			err = errors.New("application not found after update")
		}
		return h.internalError(c, err, "admin: update application failed")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"id":         updated.ID,
		"name":       updated.Name,
		"slug":       updated.Slug,
		"is_active":  updated.IsActive,
		"is_system":  updated.Slug == "system",
		"updated_at": updated.UpdatedAt,
	})
}

// RotateApplicationKey maneja POST /admin/applications/:id/rotate-key.
//
// Genera una nueva clave secreta aleatoria para la aplicación e invalida la
// anterior inmediatamente. Después de rotar la clave, todas las instancias
// de la aplicación cliente deben actualizar su configuración con la nueva clave
// o comenzarán a recibir 401 en sus peticiones.
//
// La aplicación del sistema (slug="system") no puede rotar su clave por esta vía
// para prevenir que un ataque que obtenga acceso al panel admin pueda bloquear
// el acceso total al sistema.
//
// La nueva clave se devuelve en texto plano en la respuesta y es la única
// oportunidad de verla; no se puede recuperar después.
//
// Códigos HTTP posibles:
//   - 200: nueva clave generada con campo "secret_key"
//   - 400: UUID inválido
//   - 401: JWT o X-App-Key inválidos
//   - 403: sin permiso admin.applications.write o intento de rotar app del sistema
//   - 404: aplicación no encontrada
//   - 500: error interno o fallo de generación de entropía
//
// RotateApplicationKey handles POST /admin/applications/:id/rotate-key.
//
// @Summary     Rotar clave de aplicación
// @Description Genera una nueva clave secreta para la aplicación, invalidando la anterior. La aplicación del sistema no puede rotar su clave por esta vía.
// @Tags        Aplicaciones
// @Produce     json
// @Param       X-App-Key      header   string  true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string  true  "Token JWT. Formato: Bearer {token}"
// @Param       id             path     string  true  "ID de la aplicación (UUID)"
// @Success     200  {object}  SwaggerRotateKeyResponse  "Nueva clave generada"
// @Failure     400  {object}  SwaggerErrorResponse      "ID inválido"
// @Failure     401  {object}  SwaggerErrorResponse      "No autenticado"
// @Failure     403  {object}  SwaggerErrorResponse      "Sin permiso o aplicación de sistema"
// @Failure     404  {object}  SwaggerErrorResponse      "Aplicación no encontrada"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /admin/applications/{id}/rotate-key [post]
func (h *AdminHandler) RotateApplicationKey(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid application id")
	}

	existing, err := h.appRepo.FindByID(c.Context(), id)
	if err != nil || existing == nil {
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "application not found")
	}
	if existing.Slug == "system" {
		return respondError(c, fiber.StatusForbidden, "FORBIDDEN", "the system application key cannot be rotated via API")
	}

	newKey, err := generateAppSecretKey()
	if err != nil {
		return h.internalError(c, err, "admin: generate secret key failed")
	}

	if err := h.appRepo.RotateSecretKey(c.Context(), id, newKey); err != nil {
		return h.internalError(c, err, "admin: rotate application key failed")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"secret_key": newKey,
	})
}

// generateAppSecretKey genera una clave secreta criptográficamente aleatoria
// de 32 bytes codificada en Base64 URL-safe sin padding (43 caracteres).
// Usa crypto/rand para garantizar entropía criptográfica real; si el sistema
// operativo no puede proveer entropía suficiente retorna error.
func generateAppSecretKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// mapServiceError traduce los errores del dominio de servicios admin a respuestas
// HTTP con el código de estado apropiado. Centraliza el mapeo para mantener los
// handlers limpios de lógica de traducción de errores.
//
// Mapeo de errores:
//   - ErrPasswordPolicy (wrap) -> 400 VALIDATION_ERROR  (política de contraseña no cumplida)
//   - ErrNotFound              -> 404 NOT_FOUND         (recurso no encontrado en la BD)
//   - ErrConflict              -> 409 CONFLICT          (unicidad violada: nombre/código duplicado)
//   - cualquier otro error     -> 500 INTERNAL_ERROR    (error inesperado, se loguea como ERROR)
func (h *AdminHandler) mapServiceError(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	requestID, _ := c.Locals(middleware.LocalRequestID).(string)
	msg := err.Error()
	switch {
	case isPasswordPolicyError(err):
		// La nueva contraseña no cumple las reglas de complejidad del sistema.
		h.logger.Debug("admin: password policy violation",
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", msg)
	case errors.Is(err, service.ErrNotFound):
		// El recurso solicitado no existe en la base de datos.
		h.logger.Debug("admin: resource not found",
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, service.ErrConflict):
		// Violación de restricción de unicidad (nombre, código, slug, etc.).
		h.logger.Info("admin: resource conflict",
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusConflict, "CONFLICT", "resource already exists")
	default:
		// Error inesperado: se loguea con nivel ERROR para alertar al equipo de operaciones.
		h.logger.Error("admin: internal server error",
			"error", err,
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
