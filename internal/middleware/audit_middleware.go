package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Claves de los valores almacenados en c.Locals() de Fiber.
// Todos los middlewares y handlers del sistema usan estas constantes para
// leer y escribir en el contexto de la request, garantizando consistencia
// de nombres y evitando colisiones entre paquetes.
const (
	LocalIP        = "audit_ip"         // IP del cliente después de resolver X-Forwarded-For
	LocalUserAgent = "audit_user_agent" // valor del header User-Agent del cliente
	LocalActorID   = "audit_actor_id"   // UUID del usuario autenticado (sub del JWT), como string
	LocalAppID     = "app_id"           // UUID de la aplicación cliente validada por AppKey, como string
	LocalClaims    = "jwt_claims"       // puntero *domain.Claims con los claims del JWT validado
	LocalApp       = "app"              // puntero *domain.Application de la app validada por AppKey
	LocalRequestID = "request_id"       // UUID v4 de correlación de la request (X-Request-ID)
)

// AuditContext captura los metadatos de contexto de auditoría de cada request HTTP
// y los almacena en c.Locals para que los servicios puedan incluirlos en los eventos
// de auditoría sin necesidad de acceder directamente al objeto Fiber Ctx.
//
// Debe ejecutarse al inicio de la cadena de middlewares, antes de JWTAuth y AppKey,
// para que los datos de IP y User-Agent estén disponibles incluso en requests fallidas.
//
// Lo que inyecta en c.Locals:
//   - LocalIP: IP real del cliente (X-Forwarded-For normalizado o IP directa)
//   - LocalUserAgent: header User-Agent sin modificar
//
// Nota: X-Forwarded-For puede contener una lista separada por comas cuando hay
// múltiples proxies en cadena. Se toma siempre el primer valor (IP del cliente
// original, no del proxy).
func AuditContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Resolver la IP real: preferir X-Forwarded-For sobre la IP TCP directa.
		ip := c.Get("X-Forwarded-For")
		if ip == "" {
			ip = c.IP()
		} else {
			// X-Forwarded-For puede ser una lista separada por comas; tomar el primero.
			if idx := strings.Index(ip, ","); idx != -1 {
				ip = strings.TrimSpace(ip[:idx])
			}
		}

		c.Locals(LocalIP, ip)
		c.Locals(LocalUserAgent, c.Get("User-Agent"))
		return c.Next()
	}
}
