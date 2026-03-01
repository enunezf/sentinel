package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// RequestID es un middleware de Fiber que lee o genera un ID de correlación
// único para cada request HTTP entrante.
//
// Comportamiento (en orden):
//  1. Lee el header X-Request-ID del request entrante.
//  2. Si está ausente, genera un nuevo UUID v4 con crypto/rand (via google/uuid).
//  3. Almacena el valor en c.Locals(LocalRequestID) para que los handlers y
//     middlewares posteriores puedan incluirlo en logs estructurados y eventos
//     de auditoría, permitiendo trazar una request completa en los logs.
//  4. Escribe el mismo valor en el header X-Request-ID de la respuesta para
//     que el cliente pueda correlacionar su request con los logs del servidor
//     al reportar un problema de soporte.
//
// Debe ser el primer middleware en la cadena para que LocalRequestID esté
// disponible en todos los middlewares y handlers que siguen (incluyendo
// RequestLogger y AuditContext).
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			// Generar un ID único si el cliente no proporcionó uno.
			requestID = uuid.New().String()
		}

		c.Locals(LocalRequestID, requestID)
		c.Set("X-Request-ID", requestID) // eco en la respuesta para correlación en el cliente

		return c.Next()
	}
}
