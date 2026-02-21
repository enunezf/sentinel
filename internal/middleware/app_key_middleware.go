package middleware

import (
	"github.com/gofiber/fiber/v2"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
)

// AppKey validates the X-App-Key header and injects the application into Fiber locals.
func AppKey(appRepo *postgres.ApplicationRepository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		secretKey := c.Get("X-App-Key")
		if secretKey == "" {
			return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "missing X-App-Key header")
		}

		app, err := appRepo.FindBySecretKey(c.Context(), secretKey)
		if err != nil {
			return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid or inactive application")
		}
		if app == nil || !app.IsActive {
			return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid or inactive application")
		}

		c.Locals(LocalApp, app)
		c.Locals(LocalAppID, app.ID.String())
		return c.Next()
	}
}

// GetApp extracts the application from fiber locals.
func GetApp(c *fiber.Ctx) *domain.Application {
	if v := c.Locals(LocalApp); v != nil {
		if app, ok := v.(*domain.Application); ok {
			return app
		}
	}
	return nil
}

// respondError writes a standard error JSON response.
func respondError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"error": fiber.Map{
			"code":    code,
			"message": message,
			"details": nil,
		},
	})
}
