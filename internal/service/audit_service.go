package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
)

// AuditService gestiona la persistencia asíncrona de eventos de auditoría.
// Todas las mutaciones de estado relevantes en Sentinel (login, logout, cambio de
// contraseña, creación de usuarios, asignación de roles, etc.) emiten un evento
// a través de LogEvent, que lo encola en un canal con buffer y retorna inmediatamente
// sin bloquear el flujo HTTP.
//
// Un goroutine de fondo (worker) consume el canal y persiste cada evento en PostgreSQL.
// Si el canal llega a su capacidad máxima (1000 entradas), los nuevos eventos se
// descartan con un warning en el log; la operación que los originó nunca falla por esta causa.
type AuditService struct {
	repo   *postgres.AuditRepository // persistencia de eventos en PostgreSQL
	ch     chan *domain.AuditLog      // canal asíncrono con buffer de tamaño 1000
	logger *slog.Logger              // logger estructurado para warnings y errores del worker
}

// NewAuditService crea un AuditService, inicializa el canal asíncrono con buffer de
// tamaño 1000 e inicia el goroutine worker en segundo plano.
// El worker procesa eventos secuencialmente hasta que el canal se cierre.
//
// Parámetros:
//   - repo: repositorio de auditoría de PostgreSQL.
//   - log: logger estructurado del servidor; se agrega el campo "component=audit".
func NewAuditService(repo *postgres.AuditRepository, log *slog.Logger) *AuditService {
	svc := &AuditService{
		repo:   repo,
		ch:     make(chan *domain.AuditLog, 1000), // buffer 1000 para absorber picos de tráfico
		logger: log.With("component", "audit"),
	}
	go svc.worker()
	return svc
}

// worker es el goroutine de fondo que consume el canal y persiste cada evento.
// Se ejecuta hasta que el canal se cierra (llamada a Close en el apagado graceful).
// Los errores de persistencia se registran como errores en el log pero no detienen
// el procesamiento de los siguientes eventos.
func (s *AuditService) worker() {
	for entry := range s.ch {
		ctx := context.Background()
		if err := s.repo.Insert(ctx, entry); err != nil {
			s.logger.Error("failed to insert audit log", "event_type", entry.EventType, "error", err)
		}
	}
}

// LogEvent encola un evento de auditoría para su persistencia asíncrona.
// Si el ID del evento está vacío, genera uno nuevo con UUID v4 antes de encolarlo.
//
// Comportamiento especial:
//   - Si el canal está lleno (buffer de 1000 entradas agotado), el evento se descarta
//     con un warning en el log. La operación que llamó a LogEvent nunca bloquea.
//   - Esta función nunca retorna error: los fallos de auditoría no deben impedir
//     que las operaciones de negocio (login, cambio de contraseña, etc.) respondan.
//
// Parámetros:
//   - entry: puntero al evento de auditoría a registrar. El campo ID se rellena
//     automáticamente si es el UUID cero.
func (s *AuditService) LogEvent(entry *domain.AuditLog) {
	if entry.ID == (uuid.UUID{}) {
		entry.ID = uuid.New()
	}
	select {
	case s.ch <- entry:
		// Evento encolado correctamente.
	default:
		// Canal lleno: descartar el evento para no bloquear el handler HTTP.
		s.logger.Warn("audit channel full, dropping event", "event_type", entry.EventType)
	}
}

// Close cierra el canal de auditoría y espera a que el worker procese todos los
// eventos pendientes antes de terminar. Debe llamarse durante el apagado graceful
// del servidor para asegurar que los últimos eventos queden persistidos.
func (s *AuditService) Close() {
	close(s.ch)
}
