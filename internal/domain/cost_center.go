package domain

import (
	"time"

	"github.com/google/uuid"
)

// CostCenter represents a cost center (centro de costo) entity.
type CostCenter struct {
	ID            uuid.UUID `json:"id"`
	ApplicationID uuid.UUID `json:"application_id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
}

// UserCostCenter represents the assignment of a cost center to a user.
type UserCostCenter struct {
	UserID        uuid.UUID
	CostCenterID  uuid.UUID
	ApplicationID uuid.UUID
	GrantedBy     uuid.UUID
	ValidFrom     time.Time
	ValidUntil    *time.Time
	// Populated via join.
	CostCenterCode string
	CostCenterName string
}
