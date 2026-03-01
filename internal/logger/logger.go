// Package logger provee funciones de fábrica para crear instancias de *slog.Logger
// configuradas según los parámetros del sistema Sentinel.
//
// Se apoya en la biblioteca estándar log/slog (disponible desde Go 1.21) y permite
// seleccionar el formato de salida (JSON o texto plano), el nivel de severidad mínimo
// y el destino de escritura (stdout o stderr).
//
// Uso típico en el punto de entrada de la aplicación:
//
//	appLogger := logger.New(cfg.Logging)
//	slog.SetDefault(appLogger) // establece el logger global de slog
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/enunezf/sentinel/internal/config"
)

// New crea y retorna un *slog.Logger configurado según los parámetros de LoggingConfig.
// Es la función principal que debe usarse en producción.
//
// Comportamiento de selección de salida:
//   - cfg.Output == "stderr": escribe en os.Stderr.
//   - cualquier otro valor (incluyendo "stdout" o vacío): escribe en os.Stdout.
//
// Comportamiento de selección de formato:
//   - cfg.Format == "text": produce logs legibles por humanos (útil en desarrollo local).
//   - cualquier otro valor (incluyendo "json" o vacío): produce logs en formato JSON
//     estructurado (recomendado para producción con agregadores como Loki o CloudWatch).
//
// El nivel de severidad se resuelve con ParseLevel.
//
// Parámetros:
//   - cfg: configuración de logging con Level, Format y Output.
//
// Retorna un *slog.Logger listo para usarse.
func New(cfg config.LoggingConfig) *slog.Logger {
	var w io.Writer
	if strings.ToLower(cfg.Output) == "stderr" {
		w = os.Stderr
	} else {
		w = os.Stdout
	}
	return newWithWriter(cfg, w)
}

// NewWithWriter crea y retorna un *slog.Logger que escribe en el writer provisto
// en lugar de determinar el destino a partir de cfg.Output.
//
// Esta función es útil principalmente en tests, donde se puede pasar un *bytes.Buffer
// para capturar el output del logger sin redirigir os.Stdout ni os.Stderr.
//
// Parámetros:
//   - cfg: configuración de logging con Level y Format (Output se ignora).
//   - w: writer donde se escribirán los logs.
//
// Retorna un *slog.Logger que escribe en w.
func NewWithWriter(cfg config.LoggingConfig, w io.Writer) *slog.Logger {
	return newWithWriter(cfg, w)
}

// newWithWriter es la implementación interna compartida por New y NewWithWriter.
// Construye el handler de slog apropiado según el formato configurado
// y retorna un logger configurado con ese handler.
//
// Parámetros:
//   - cfg: configuración de logging con Level y Format.
//   - w: writer donde se escribirán los logs.
func newWithWriter(cfg config.LoggingConfig, w io.Writer) *slog.Logger {
	level := ParseLevel(cfg.Level)

	opts := &slog.HandlerOptions{
		// Level establece el umbral mínimo de severidad; mensajes por debajo se descartan.
		Level: level,
	}

	var handler slog.Handler

	if strings.ToLower(cfg.Format) == "text" {
		handler = slog.NewTextHandler(w, opts)
	} else {
		// Por defecto se usa JSON. Cubre el valor "json" y cualquier formato no reconocido.
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(handler)
}

// ParseLevel convierte un string de nivel de severidad al tipo slog.Level correspondiente.
// La comparación es insensible a mayúsculas/minúsculas y elimina espacios en los extremos.
//
// Mapeo:
//   - "debug"           -> slog.LevelDebug  (-4)
//   - "warn"/"warning"  -> slog.LevelWarn   (4)
//   - "error"           -> slog.LevelError  (8)
//   - cualquier otro    -> slog.LevelInfo   (0)  (valor por defecto seguro)
//
// Parámetros:
//   - level: cadena con el nivel de severidad deseado.
//
// Retorna el slog.Level correspondiente, o slog.LevelInfo si el valor no es reconocido.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithComponent retorna un logger derivado del logger base con el atributo estructurado
// "component" fijo en cada mensaje que emita.
//
// Esto permite identificar de qué capa del sistema proviene cada log sin necesidad
// de repetir el campo "component" en cada llamada individual al logger.
//
// Uso típico:
//
//	repoLogger := logger.WithComponent(appLogger, "user_repository")
//	repoLogger.Info("usuario creado", "user_id", id)
//	// Produce: {"level":"INFO","component":"user_repository","msg":"usuario creado","user_id":"..."}
//
// Parámetros:
//   - logger: logger base del que se deriva el nuevo logger hijo.
//   - component: nombre del componente o capa que se registrará en cada mensaje.
//
// Retorna un nuevo *slog.Logger con el atributo "component" preconfigurado.
func WithComponent(logger *slog.Logger, component string) *slog.Logger {
	return logger.With("component", component)
}
