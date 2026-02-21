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
