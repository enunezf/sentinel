package middleware

import "github.com/gofiber/fiber/v2"

// SecurityHeaders agrega los headers de seguridad HTTP obligatorios a todas las
// respuestas del servidor. Debe estar al inicio de la cadena de middlewares para
// garantizar que los headers se emitan incluso en respuestas de error.
//
// Headers que inyecta:
//   - Strict-Transport-Security: max-age=31536000; includeSubDomains
//     Instruye al navegador a usar HTTPS por 1 año e incluir subdominios.
//     Previene ataques de downgrade SSL/TLS (HSTS).
//
//   - X-Content-Type-Options: nosniff
//     Impide que el navegador infiera el tipo MIME del contenido, previniendo
//     ataques de MIME sniffing que podrían ejecutar contenido malicioso.
//
//   - X-Frame-Options: DENY
//     Prohíbe que la respuesta se muestre dentro de un iframe/frame/object,
//     previniendo ataques de clickjacking.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Forzar HTTPS durante 1 año en el navegador, incluyendo subdominios.
		c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		// Prevenir ataques de MIME sniffing.
		c.Set("X-Content-Type-Options", "nosniff")
		// Prevenir clickjacking bloqueando el uso de iframes.
		c.Set("X-Frame-Options", "DENY")
		return c.Next()
	}
}
