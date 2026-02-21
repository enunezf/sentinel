package middleware

import (
	"github.com/gofiber/fiber/v2"

	"github.com/enunezf/sentinel/internal/service"
)

// RequirePermission returns a middleware that checks if the authenticated user has the required permission.
func RequirePermission(authzSvc *service.AuthzService, permCode string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims := GetClaims(c)
		if claims == nil {
			return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "missing authentication")
		}

		allowed, err := authzSvc.HasPermission(c.Context(), claims, permCode)
		if err != nil {
			return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "authorization check failed")
		}
		if !allowed {
			return respondError(c, fiber.StatusForbidden, "FORBIDDEN", "insufficient permissions")
		}

		return c.Next()
	}
}
