package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// ContextKeys for values stored in Fiber locals.
const (
	LocalIP        = "audit_ip"
	LocalUserAgent = "audit_user_agent"
	LocalActorID   = "audit_actor_id"
	LocalAppID     = "app_id"
	LocalClaims    = "jwt_claims"
	LocalApp       = "app"
)

// AuditContext captures request metadata for audit logging.
func AuditContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Capture IP: prefer X-Forwarded-For, fallback to remote IP.
		ip := c.Get("X-Forwarded-For")
		if ip == "" {
			ip = c.IP()
		} else {
			// X-Forwarded-For can be a comma-separated list; take the first.
			if idx := strings.Index(ip, ","); idx != -1 {
				ip = strings.TrimSpace(ip[:idx])
			}
		}

		c.Locals(LocalIP, ip)
		c.Locals(LocalUserAgent, c.Get("User-Agent"))
		return c.Next()
	}
}
