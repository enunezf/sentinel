// Package middleware contiene los middlewares HTTP de Sentinel para Fiber v2.
// Cada middleware intercepta la cadena de handlers antes de llegar al handler final,
// realiza su trabajo específico (validación, inyección de contexto, logging) y
// decide si continúa con c.Next() o corta la cadena retornando un error.
//
// Los datos que un middleware necesita compartir con los handlers posteriores
// se almacenan en c.Locals() usando las constantes LocalXxx definidas en
// audit_middleware.go como claves tipadas.
package middleware

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
)

// AppKey valida el header X-App-Key en cada request. Intercepta todas las
// peticiones que tengan este middleware en su cadena de Fiber.
//
// Comportamiento:
//   - Si X-App-Key está ausente -> 401 y corta la cadena.
//   - Si la clave no corresponde a ninguna aplicación en BD -> 401 y corta.
//   - Si la aplicación existe pero está inactiva (is_active=false) -> 401 y corta.
//   - Si es válida -> inyecta *domain.Application en c.Locals(LocalApp) y
//     el ID como string en c.Locals(LocalAppID), luego llama c.Next().
//
// Uso en el router: se aplica a todos los endpoints excepto GET /health
// y GET /.well-known/jwks.json.
func AppKey(appRepo *postgres.ApplicationRepository, log *slog.Logger) fiber.Handler {
	logger := log.With("component", "app_key_middleware")
	return func(c *fiber.Ctx) error {
		requestID, _ := c.Locals(LocalRequestID).(string)

		secretKey := c.Get("X-App-Key")
		if secretKey == "" {
			logger.Warn("missing X-App-Key header",
				"ip", c.IP(),
				"request_id", requestID,
			)
			return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "missing X-App-Key header")
		}

		app, err := appRepo.FindBySecretKey(c.Context(), secretKey)
		if err != nil {
			// Error de BD o clave no encontrada: se loguea como Warn para detectar ataques.
			logger.Warn("invalid X-App-Key",
				"ip", c.IP(),
				"request_id", requestID,
			)
			return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid or inactive application")
		}
		if app == nil || !app.IsActive {
			// Clave válida pero la aplicación fue desactivada manualmente.
			logger.Warn("inactive or not found application for X-App-Key",
				"ip", c.IP(),
				"request_id", requestID,
			)
			return respondError(c, fiber.StatusUnauthorized, "APPLICATION_NOT_FOUND", "invalid or inactive application")
		}

		logger.Debug("X-App-Key validated",
			"app_slug", app.Slug,
			"request_id", requestID,
		)

		// Inyecta la aplicación en Locals para que los handlers puedan acceder a
		// app.ID, app.Slug, etc. sin volver a consultar la base de datos.
		c.Locals(LocalApp, app)
		c.Locals(LocalAppID, app.ID.String())
		return c.Next()
	}
}

// GetApp extrae el objeto *domain.Application inyectado por el middleware AppKey
// desde c.Locals. Retorna nil si el middleware no se ejecutó (endpoint sin AppKey)
// o si el valor almacenado tiene un tipo inesperado.
// Los handlers deben verificar nil antes de usar la aplicación.
func GetApp(c *fiber.Ctx) *domain.Application {
	if v := c.Locals(LocalApp); v != nil {
		if app, ok := v.(*domain.Application); ok {
			return app
		}
	}
	return nil
}

// respondError escribe una respuesta de error JSON con el formato estándar de la API.
// Esta función es privada al paquete middleware y duplica intencionalmente la del
// paquete handler para evitar dependencias circulares entre paquetes.
func respondError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"error": fiber.Map{
			"code":    code,
			"message": message,
			"details": nil,
		},
	})
}
