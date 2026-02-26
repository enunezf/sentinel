package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/enunezf/sentinel/internal/middleware"
	"github.com/enunezf/sentinel/internal/service"
)

// AuthzHandler handles authorization endpoints.
type AuthzHandler struct {
	authzSvc *service.AuthzService
}

// NewAuthzHandler creates a new AuthzHandler.
func NewAuthzHandler(authzSvc *service.AuthzService) *AuthzHandler {
	return &AuthzHandler{authzSvc: authzSvc}
}

// verifyRequest is the POST /authz/verify request body.
type verifyRequest struct {
	Permission   string `json:"permission"`
	CostCenterID string `json:"cost_center_id"`
}

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
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "authorization check failed")
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

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
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to get permissions")
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

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
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to get permissions map")
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

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
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "failed to get map version")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"application":  app.Slug,
		"version":      version,
		"generated_at": generatedAt,
	})
}
