package middleware

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/enunezf/sentinel/internal/service"
)

// RequirePermission genera un middleware de Fiber que verifica si el usuario
// autenticado tiene el permiso RBAC identificado por permCode.
//
// Debe ejecutarse siempre DESPUÉS del middleware JWTAuth en la cadena, porque
// necesita los claims inyectados en c.Locals(LocalClaims).
//
// Comportamiento:
//   - Si no hay claims en Locals (JWTAuth no ejecutó o fue omitido) -> 401 y corta.
//   - Si la consulta RBAC falla (error de BD o caché) -> 500 y corta.
//   - Si el usuario NO tiene el permiso -> 403 FORBIDDEN y corta.
//   - Si el usuario SÍ tiene el permiso -> llama c.Next() para continuar la cadena.
//
// Uso en el router: se instancia una vez por ruta con el código de permiso específico.
// Ejemplo:
//
//	admin.Get("/users", middleware.RequirePermission(authzSvc, "admin.users.read", log), h.ListUsers)
func RequirePermission(authzSvc *service.AuthzService, permCode string, log *slog.Logger) fiber.Handler {
	logger := log.With("component", "permission_middleware")
	return func(c *fiber.Ctx) error {
		requestID, _ := c.Locals(LocalRequestID).(string)

		claims := GetClaims(c)
		if claims == nil {
			// No debería ocurrir si JWTAuth precede este middleware en el router.
			return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "missing authentication")
		}

		allowed, err := authzSvc.HasPermission(c.Context(), claims, permCode)
		if err != nil {
			// Error inesperado en la capa de autorización (BD, Redis, etc.).
			logger.Error("permission check failed",
				"error", err,
				"user_id", claims.Sub,
				"permission_code", permCode,
				"request_id", requestID,
			)
			return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "authorization check failed")
		}
		if !allowed {
			// El usuario no tiene el permiso requerido. Se loguea como Info para
			// facilitar la detección de intentos de acceso no autorizado.
			logger.Info("permission denied",
				"user_id", claims.Sub,
				"permission_code", permCode,
				"request_id", requestID,
			)
			return respondError(c, fiber.StatusForbidden, "FORBIDDEN", "insufficient permissions")
		}

		logger.Debug("permission granted",
			"user_id", claims.Sub,
			"permission_code", permCode,
			"request_id", requestID,
		)

		return c.Next()
	}
}
