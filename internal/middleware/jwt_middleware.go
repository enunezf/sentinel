package middleware

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/token"
)

// JWTAuth valida el Bearer token del header Authorization usando el algoritmo RS256.
// Intercepta todas las peticiones que tengan este middleware en su cadena de Fiber.
//
// Comportamiento:
//   - Si el header Authorization está ausente -> 401 TOKEN_INVALID y corta la cadena.
//   - Si el formato no es "Bearer <token>" -> 401 TOKEN_INVALID y corta.
//   - Si el token está expirado (jwt.ErrTokenExpired) -> 401 TOKEN_EXPIRED y corta.
//   - Si el token tiene firma inválida u otro error -> 401 TOKEN_INVALID y corta.
//   - Si el token es válido -> inyecta *domain.Claims en c.Locals(LocalClaims) y
//     el sub (user_id) en c.Locals(LocalActorID), luego llama c.Next().
//
// Seguridad: nunca loguea el contenido del token para evitar filtrar claims
// sensibles en los logs.
//
// Uso en el router: se aplica a todos los endpoints que requieren autenticación
// de usuario (rutas /auth/logout, /auth/change-password, /authz/* y /admin/*).
func JWTAuth(tokenMgr *token.Manager, log *slog.Logger) fiber.Handler {
	logger := log.With("component", "jwt_middleware")
	return func(c *fiber.Ctx) error {
		requestID, _ := c.Locals(LocalRequestID).(string)

		authHeader := c.Get("Authorization")
		if authHeader == "" {
			logger.Warn("missing Authorization header",
				"ip", c.IP(),
				"request_id", requestID,
			)
			return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "missing Authorization header")
		}

		// El header debe tener exactamente dos partes: el esquema y el token.
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			logger.Warn("invalid Authorization header format",
				"ip", c.IP(),
				"request_id", requestID,
			)
			return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "invalid Authorization header format")
		}

		tokenStr := parts[1]
		claims, err := tokenMgr.ValidateToken(tokenStr)
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				// Token expirado: el cliente debe usar su refresh token para obtener uno nuevo.
				logger.Warn("expired JWT token",
					"ip", c.IP(),
					"request_id", requestID,
				)
				return respondError(c, fiber.StatusUnauthorized, "TOKEN_EXPIRED", "access token has expired")
			}
			// Token malformado, firma inválida o clave incorrecta.
			logger.Warn("invalid JWT token",
				"ip", c.IP(),
				"request_id", requestID,
			)
			return respondError(c, fiber.StatusUnauthorized, "TOKEN_INVALID", "invalid access token")
		}

		logger.Debug("JWT token validated",
			"request_id", requestID,
		)

		// Inyecta los claims en Locals para que los handlers y middlewares posteriores
		// (ej. RequirePermission) puedan acceder al user_id sin re-validar el token.
		c.Locals(LocalClaims, claims)
		c.Locals(LocalActorID, claims.Sub) // sub es el UUID del usuario autenticado
		return c.Next()
	}
}

// GetClaims extrae los *domain.Claims inyectados por el middleware JWTAuth
// desde c.Locals. Retorna nil si el middleware no se ejecutó (endpoint público)
// o si el valor tiene un tipo inesperado.
// Los handlers deben verificar nil antes de acceder a los claims.
func GetClaims(c *fiber.Ctx) *domain.Claims {
	if v := c.Locals(LocalClaims); v != nil {
		if claims, ok := v.(*domain.Claims); ok {
			return claims
		}
	}
	return nil
}
