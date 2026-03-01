package handler

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/enunezf/sentinel/internal/middleware"
	"github.com/enunezf/sentinel/internal/service"
)

// AuthzHandler agrupa los handlers de los endpoints de autorización RBAC:
// verificación de permisos, consulta de permisos propios y mapa de permisos
// firmado para consumo por backends externos.
type AuthzHandler struct {
	authzSvc *service.AuthzService // servicio que implementa la lógica de autorización RBAC con caché
	logger   *slog.Logger          // logger estructurado con atributo component="authz"
}

// NewAuthzHandler construye un AuthzHandler inyectando sus dependencias.
// El logger recibido se enriquece con el atributo component="authz".
func NewAuthzHandler(authzSvc *service.AuthzService, log *slog.Logger) *AuthzHandler {
	return &AuthzHandler{
		authzSvc: authzSvc,
		logger:   log.With("component", "authz"),
	}
}

// verifyRequest es el cuerpo de la petición POST /authz/verify.
type verifyRequest struct {
	Permission   string `json:"permission"`    // código del permiso a verificar (ej: "erp.reportes.read")
	CostCenterID string `json:"cost_center_id"` // UUID del centro de costo para permisos con alcance restringido (opcional)
}

// Verify maneja POST /authz/verify.
//
// Verifica si el usuario autenticado (identificado por los claims del JWT)
// tiene un permiso específico. Opcionalmente, si se proporciona cost_center_id,
// la verificación incluye si el usuario tiene acceso en ese centro de costo.
//
// Este endpoint es el punto de integración principal para backends consumidores
// que necesitan verificar permisos en tiempo real. Para verificaciones frecuentes
// se recomienda usar el mapa de permisos (GET /authz/permissions-map) en su lugar,
// ya que ese endpoint es cacheable y reduce la carga sobre Sentinel.
//
// Registra el evento de verificación en el log de auditoría (resultado allowed/denied).
//
// Códigos HTTP posibles:
//   - 200: responde {"allowed": true/false, "user_id": ..., "permission": ..., "evaluated_at": ...}
//   - 400: body inválido o campo permission vacío
//   - 401: JWT ausente o inválido
//   - 500: error interno del motor de autorización
//
// Verify handles POST /authz/verify.
//
// @Summary     Verificar permiso
// @Description Verifica si el usuario autenticado tiene un permiso específico, opcionalmente en un centro de costo.
// @Tags        Autorización
// @Accept      json
// @Produce     json
// @Param       X-App-Key      header   string                          true  "Clave secreta de la aplicación"
// @Param       Authorization  header   string                          true  "Token JWT. Formato: Bearer {token}"
// @Param       body           body     SwaggerVerifyPermissionRequest  true  "Permiso a verificar"
// @Success     200            {object} SwaggerVerifyResponse           "Resultado de la verificación"
// @Failure     400            {object} SwaggerErrorResponse            "Datos inválidos"
// @Failure     401            {object} SwaggerErrorResponse            "No autenticado"
// @Security    BearerAuth
// @Security    AppKeyAuth
// @Router      /authz/verify [post]
func (h *AuthzHandler) Verify(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "missing authentication")
	}

	var req verifyRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
	}
	if req.Permission == "" {
		return respondError(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "permission is required")
	}

	resp, err := h.authzSvc.Verify(c.Context(), claims, service.VerifyRequest{
		Permission:   req.Permission,
		CostCenterID: req.CostCenterID,
	}, getIP(c), c.Get("User-Agent"))
	if err != nil {
		requestID, _ := c.Locals(middleware.LocalRequestID).(string)
		h.logger.Error("authz verify: internal error",
			"error", err,
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "authorization check failed")
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// MePermissions maneja GET /authz/me/permissions.
//
// Retorna el perfil de autorización completo del usuario autenticado:
// roles activos, permisos efectivos (sumando los de los roles y los directos),
// centros de costo asignados y roles temporales con vigencia activa.
//
// El frontend usa este endpoint al inicializar la sesión para construir el
// menú y controlar la visibilidad de elementos de la UI según los permisos
// del usuario actual, sin necesidad de hacer llamadas adicionales por cada
// elemento que se quiera condicionar.
//
// Nota: este endpoint no requiere X-App-Key (ver anotaciones Swagger); solo
// necesita el JWT Bearer. La aplicación se infiere del campo "app" en el token.
//
// Códigos HTTP posibles:
//   - 200: perfil de autorización del usuario
//   - 401: JWT ausente o inválido
//   - 500: error interno al cargar los permisos
//
// MePermissions handles GET /authz/me/permissions.
//
// @Summary     Mis permisos
// @Description Retorna todos los roles, permisos y centros de costo del usuario autenticado para la aplicación actual.
// @Tags        Autorización
// @Produce     json
// @Param       Authorization  header   string                        true  "Token JWT. Formato: Bearer {token}"
// @Success     200            {object} SwaggerMePermissionsResponse  "Permisos del usuario"
// @Failure     401            {object} SwaggerErrorResponse          "No autenticado"
// @Security    BearerAuth
// @Router      /authz/me/permissions [get]
func (h *AuthzHandler) MePermissions(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "missing authentication")
	}

	resp, err := h.authzSvc.GetUserPermissions(c.Context(), claims)
	if err != nil {
		requestID, _ := c.Locals(middleware.LocalRequestID).(string)
		h.logger.Error("authz me/permissions: internal error",
			"error", err,
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to get permissions")
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// PermissionsMap maneja GET /authz/permissions-map.
//
// Retorna el mapa completo de permisos de la aplicación identificada por X-App-Key.
// El mapa contiene todos los permisos de la aplicación con los roles que los tienen
// asignados y la lista de centros de costo disponibles.
//
// El mapa está firmado con RSA-SHA256 (campo "signature") usando el JSON canónico
// (claves ordenadas lexicográficamente, sin espacios) para que los backends
// consumidores puedan verificar la integridad del mapa con la clave pública
// disponible en /.well-known/jwks.json.
//
// Este endpoint es ideal para backends que cargan el mapa al arrancar y lo
// cachean localmente, verificando periódicamente la versión con
// GET /authz/permissions-map/version para detectar cambios.
//
// Códigos HTTP posibles:
//   - 200: mapa de permisos firmado
//   - 401: X-App-Key inválida o ausente
//   - 500: error al generar el mapa o la firma
//
// PermissionsMap handles GET /authz/permissions-map.
//
// @Summary     Mapa de permisos
// @Description Retorna el mapa completo de permisos de la aplicación, firmado con RSA-SHA256.
// @Tags        Autorización
// @Produce     json
// @Param       X-App-Key  header   string                  true  "Clave secreta de la aplicación"
// @Success     200        {object} SwaggerPermissionsMapResponse  "Mapa de permisos firmado con RSA-SHA256"
// @Failure     401        {object} SwaggerErrorResponse    "Aplicación no encontrada"
// @Security    AppKeyAuth
// @Router      /authz/permissions-map [get]
func (h *AuthzHandler) PermissionsMap(c *fiber.Ctx) error {
	app := middleware.GetApp(c)
	if app == nil {
		return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid application")
	}

	resp, err := h.authzSvc.GetPermissionsMap(c.Context(), app.Slug)
	if err != nil {
		requestID, _ := c.Locals(middleware.LocalRequestID).(string)
		h.logger.Error("authz permissions-map: internal error",
			"error", err,
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to get permissions map")
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// PermissionsMapVersion maneja GET /authz/permissions-map/version.
//
// Retorna solo la versión actual (hash SHA256) del mapa de permisos y la fecha
// de generación, sin devolver el mapa completo. Esto permite a los backends
// consumidores verificar si el mapa que tienen cacheado sigue vigente sin
// descargar el mapa completo en cada poll.
//
// El flujo recomendado para backends consumidores es:
//  1. Al arrancar: GET /authz/permissions-map → cargar mapa y cachear versión.
//  2. Periódicamente: GET /authz/permissions-map/version → comparar versión.
//  3. Si la versión cambió: GET /authz/permissions-map → actualizar caché.
//
// Códigos HTTP posibles:
//   - 200: {"application": slug, "version": hash, "generated_at": timestamp}
//   - 401: X-App-Key inválida o ausente
//   - 500: error al calcular la versión
//
// PermissionsMapVersion handles GET /authz/permissions-map/version.
//
// @Summary     Versión del mapa de permisos
// @Description Retorna la versión actual (hash) del mapa de permisos de la aplicación.
// @Tags        Autorización
// @Produce     json
// @Param       X-App-Key  header   string                                true  "Clave secreta de la aplicación"
// @Success     200        {object} SwaggerPermissionsMapVersionResponse  "Versión del mapa de permisos"
// @Failure     401        {object} SwaggerErrorResponse                  "Aplicación no encontrada"
// @Security    AppKeyAuth
// @Router      /authz/permissions-map/version [get]
func (h *AuthzHandler) PermissionsMapVersion(c *fiber.Ctx) error {
	app := middleware.GetApp(c)
	if app == nil {
		return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid application")
	}

	version, generatedAt, err := h.authzSvc.GetPermissionsMapVersion(c.Context(), app.Slug)
	if err != nil {
		requestID, _ := c.Locals(middleware.LocalRequestID).(string)
		h.logger.Error("authz permissions-map/version: internal error",
			"error", err,
			"request_id", requestID,
		)
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to get map version")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"application":  app.Slug,
		"version":      version,
		"generated_at": generatedAt,
	})
}
