package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// SEGURIDAD: nunca loguear datos sensibles (contraseñas, tokens, claves, header Authorization).

// RequestLogger es un middleware de Fiber que emite una entrada de log estructurada
// (slog) por cada request HTTP, después de que la respuesta fue enviada al cliente.
//
// Reemplaza el middleware de logging incluido en github.com/gofiber/fiber/v2/middleware/logger
// y agrega las siguientes capacidades:
//   - Propagación del ID de correlación (LocalRequestID) establecido por el middleware RequestID.
//   - ID del usuario autenticado (LocalActorID) cuando está disponible en Locals.
//   - ID de la aplicación cliente (LocalAppID) cuando fue validado por AppKey.
//   - Nivel de log adaptativo según el status HTTP y la ruta:
//     DEBUG para /health y /swagger* (alta frecuencia, bajo valor de señal)
//     ERROR para status >= 500
//     WARN  para status >= 400
//     INFO  para todo lo demás
//   - Evaluación perezosa con slog.Logger.Enabled para evitar alocar el record
//     cuando el nivel configurado descarta el log.
//
// El logger se inyecta como dependencia; no hay estado global en el paquete.
//
// Debe estar en la cadena de middlewares después de RequestID para que
// LocalRequestID ya esté disponible.
func RequestLogger(log *slog.Logger) fiber.Handler {
	httpLogger := log.With("component", "http")

	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Delega al siguiente handler de la cadena y captura cualquier error retornado.
		chainErr := c.Next()

		latencyMs := float64(time.Since(start).Microseconds()) / 1000.0
		status := c.Response().StatusCode()
		path := c.Path()
		method := c.Method()

		// Determinar el nivel efectivo según status y ruta.
		level := resolveLevel(status, path)

		// Evaluación perezosa: si el nivel está por debajo del mínimo configurado,
		// omitir completamente la emisión del log sin alocar estructuras.
		if !httpLogger.Enabled(c.Context(), level) {
			return chainErr
		}

		// Atributos base siempre presentes en todas las entradas de log.
		attrs := []any{
			"method", method,
			"path", path,
			"status", status,
			"latency_ms", latencyMs,
			"ip", clientIP(c),
		}

		// ID de correlación — presente cuando el middleware RequestID precede este.
		if rid, ok := c.Locals(LocalRequestID).(string); ok && rid != "" {
			attrs = append(attrs, "request_id", rid)
		}

		// ID de usuario — presente solo en endpoints autenticados.
		if uid, ok := c.Locals(LocalActorID).(string); ok && uid != "" {
			attrs = append(attrs, "user_id", uid)
		}

		// ID de aplicación — presente solo cuando X-App-Key fue validado.
		if aid, ok := c.Locals(LocalAppID).(string); ok && aid != "" {
			attrs = append(attrs, "app_id", aid)
		}

		httpLogger.Log(c.Context(), level, "HTTP request", attrs...)

		return chainErr
	}
}

// resolveLevel determina el nivel de log slog apropiado para un request según
// su status HTTP y su ruta.
//
// Reglas (en orden de prioridad):
//   - Rutas /health y /swagger* -> DEBUG (alta frecuencia, bajo valor de señal)
//   - status >= 500             -> ERROR (errores del servidor que requieren atención)
//   - status >= 400             -> WARN  (errores del cliente, posibles ataques)
//   - cualquier otro status     -> INFO  (requests normales exitosas)
func resolveLevel(status int, path string) slog.Level {
	if path == "/health" || strings.HasPrefix(path, "/swagger") {
		return slog.LevelDebug
	}
	if status >= 500 {
		return slog.LevelError
	}
	if status >= 400 {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

// clientIP extrae la IP real del cliente prefiriendo X-Forwarded-For cuando está
// presente. Replica la misma lógica que AuditContext para mantener consistencia
// en la resolución de IP a través de todo el sistema.
func clientIP(c *fiber.Ctx) string {
	ip := c.Get("X-Forwarded-For")
	if ip == "" {
		return c.IP()
	}
	// X-Forwarded-For puede ser una lista separada por comas; tomar el primero (cliente original).
	if idx := strings.Index(ip, ","); idx != -1 {
		return strings.TrimSpace(ip[:idx])
	}
	return strings.TrimSpace(ip)
}
