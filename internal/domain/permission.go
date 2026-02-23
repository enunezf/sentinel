package domain

import (
	"time"

	"github.com/google/uuid"
)

// ScopeType represents the scope classification of a permission.
type ScopeType string

const (
	ScopeTypeGlobal   ScopeType = "global"
	ScopeTypeModule   ScopeType = "module"
	ScopeTypeResource ScopeType = "resource"
	ScopeTypeAction   ScopeType = "action"
)

// IsValidScopeType returns true if st is one of the allowed scope types.
func IsValidScopeType(st string) bool {
	switch ScopeType(st) {
	case ScopeTypeGlobal, ScopeTypeModule, ScopeTypeResource, ScopeTypeAction:
		return true
	}
	return false
}

// Permission represents an authorization permission code for an application.
type Permission struct {
	ID            uuid.UUID `json:"id"`
	ApplicationID uuid.UUID `json:"application_id"`
	Code          string    `json:"code"`
	Description   string    `json:"description"`
	ScopeType     ScopeType `json:"scope_type"`
	CreatedAt     time.Time `json:"created_at"`
}

// UserPermission represents a direct permission assignment to a user.
type UserPermission struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	PermissionID  uuid.UUID
	ApplicationID uuid.UUID
	GrantedBy     uuid.UUID
	ValidFrom     time.Time
	ValidUntil    *time.Time
	IsActive      bool
	CreatedAt     time.Time
	// Populated via join.
	PermissionCode string
}
