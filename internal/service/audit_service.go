package service

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/enunezf/sentinel/internal/domain"
	"github.com/enunezf/sentinel/internal/repository/postgres"
)

// AuditService handles asynchronous audit log persistence.
type AuditService struct {
	repo *postgres.AuditRepository
	ch   chan *domain.AuditLog
}

// NewAuditService creates an AuditService with a buffered channel and starts the worker.
func NewAuditService(repo *postgres.AuditRepository) *AuditService {
	svc := &AuditService{
		repo: repo,
		ch:   make(chan *domain.AuditLog, 1000),
	}
	go svc.worker()
	return svc
}

// worker reads from the channel and persists audit logs.
func (s *AuditService) worker() {
	for entry := range s.ch {
		ctx := context.Background()
		if err := s.repo.Insert(ctx, entry); err != nil {
			log.Printf("AUDIT_ERROR: failed to insert audit log event=%s err=%v", entry.EventType, err)
		}
	}
}

// LogEvent submits an audit log entry asynchronously.
// If the channel is full, the event is dropped and an error is logged.
func (s *AuditService) LogEvent(entry *domain.AuditLog) {
	if entry.ID == (uuid.UUID{}) {
		entry.ID = uuid.New()
	}
	select {
	case s.ch <- entry:
	default:
		log.Printf("AUDIT_WARN: audit channel full, dropping event=%s", entry.EventType)
	}
}

// Close drains and closes the audit channel. Call on graceful shutdown.
func (s *AuditService) Close() {
	close(s.ch)
}
