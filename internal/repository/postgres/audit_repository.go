package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/enunezf/sentinel/internal/domain"
)

// AuditRepository handles persistence of AuditLog entries (INSERT and SELECT only).
type AuditRepository struct {
	db *pgxpool.Pool
}

// NewAuditRepository creates a new AuditRepository.
func NewAuditRepository(db *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{db: db}
}

// Insert writes an audit log entry to the database.
func (r *AuditRepository) Insert(ctx context.Context, log *domain.AuditLog) error {
	var oldValueJSON, newValueJSON []byte
	var err error
	if log.OldValue != nil {
		oldValueJSON, err = json.Marshal(log.OldValue)
		if err != nil {
			return fmt.Errorf("audit_repo: marshal old_value: %w", err)
		}
	}
	if log.NewValue != nil {
		newValueJSON, err = json.Marshal(log.NewValue)
		if err != nil {
			return fmt.Errorf("audit_repo: marshal new_value: %w", err)
		}
	}

	var ipAddr interface{}
	if log.IPAddress != "" {
		ipAddr = log.IPAddress
	}

	const q = `
		INSERT INTO audit_logs (
			id, event_type, application_id, user_id, actor_id,
			resource_type, resource_id, old_value, new_value,
			ip_address, user_agent, success, error_message, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NOW())`

	_, err = r.db.Exec(ctx, q,
		log.ID, string(log.EventType), log.ApplicationID, log.UserID, log.ActorID,
		log.ResourceType, log.ResourceID,
		nullableJSON(oldValueJSON), nullableJSON(newValueJSON),
		ipAddr, log.UserAgent, log.Success, nullableString(log.ErrorMessage),
	)
	if err != nil {
		return fmt.Errorf("audit_repo: insert: %w", err)
	}
	return nil
}

func nullableJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// AuditFilter defines optional filters for listing audit logs.
type AuditFilter struct {
	UserID        *uuid.UUID
	ActorID       *uuid.UUID
	EventType     string
	FromDate      *time.Time
	ToDate        *time.Time
	ApplicationID *uuid.UUID
	Success       *bool
	Page          int
	PageSize      int
}

// List returns paginated audit log entries matching the filter.
func (r *AuditRepository) List(ctx context.Context, filter AuditFilter) ([]*domain.AuditLog, int, error) {
	args := []interface{}{}
	where := []string{}
	idx := 1

	if filter.UserID != nil {
		where = append(where, fmt.Sprintf("user_id = $%d", idx))
		args = append(args, *filter.UserID)
		idx++
	}
	if filter.ActorID != nil {
		where = append(where, fmt.Sprintf("actor_id = $%d", idx))
		args = append(args, *filter.ActorID)
		idx++
	}
	if filter.EventType != "" {
		where = append(where, fmt.Sprintf("event_type = $%d", idx))
		args = append(args, filter.EventType)
		idx++
	}
	if filter.FromDate != nil {
		where = append(where, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, *filter.FromDate)
		idx++
	}
	if filter.ToDate != nil {
		where = append(where, fmt.Sprintf("created_at <= $%d", idx))
		args = append(args, *filter.ToDate)
		idx++
	}
	if filter.ApplicationID != nil {
		where = append(where, fmt.Sprintf("application_id = $%d", idx))
		args = append(args, *filter.ApplicationID)
		idx++
	}
	if filter.Success != nil {
		where = append(where, fmt.Sprintf("success = $%d", idx))
		args = append(args, *filter.Success)
		idx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs `+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit_repo: list count: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize
	dataQ := `
		SELECT id, event_type, application_id, user_id, actor_id,
		       resource_type, resource_id, old_value, new_value,
		       ip_address::text, user_agent, success, error_message, created_at
		FROM audit_logs ` + whereClause +
		fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, filter.PageSize, offset)

	rows, err := r.db.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit_repo: list query: %w", err)
	}
	defer rows.Close()

	logs := make([]*domain.AuditLog, 0)
	for rows.Next() {
		var l domain.AuditLog
		var oldValRaw, newValRaw []byte
		var ipAddr *string
		var errorMsg *string
		err := rows.Scan(
			&l.ID, &l.EventType, &l.ApplicationID, &l.UserID, &l.ActorID,
			&l.ResourceType, &l.ResourceID, &oldValRaw, &newValRaw,
			&ipAddr, &l.UserAgent, &l.Success, &errorMsg, &l.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("audit_repo: scan: %w", err)
		}
		if ipAddr != nil {
			l.IPAddress = *ipAddr
		}
		if errorMsg != nil {
			l.ErrorMessage = *errorMsg
		}
		if len(oldValRaw) > 0 {
			_ = json.Unmarshal(oldValRaw, &l.OldValue)
		}
		if len(newValRaw) > 0 {
			_ = json.Unmarshal(newValRaw, &l.NewValue)
		}
		logs = append(logs, &l)
	}
	return logs, total, rows.Err()
}
