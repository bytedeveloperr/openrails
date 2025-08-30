package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Role struct {
	bun.BaseModel `bun:"table:roles,alias:r"`

	ID          uuid.UUID `bun:"id,pk,type:uuid" json:"id" binding:"required"`
	Name        string    `json:"name" binding:"required"`
	Slug        string    `json:"slug" binding:"required"`
	Description string    `json:"description"`

	CreatedAt time.Time `bun:",notnull,type:timestamptz,default:current_timestamp" json:"created_at" binding:"required"`
	UpdatedAt time.Time `bun:",notnull,type:timestamptz,default:current_timestamp" json:"updated_at" binding:"required"`
	DeletedAt time.Time `bun:",soft_delete,type:timestamptz,nullzero" json:"-"`
}

// UserRoleGrant represents the current role assignment for a user
// Simple model with a single AutoExpiresAt field that gets extended by purchases.
// NOTE: Expired grants are hard-deleted (no soft delete / DeletedAt column).
type UserRoleGrant struct {
	bun.BaseModel `bun:"table:user_role_grants,alias:urg"`

	ID     uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	UserID uuid.UUID `bun:"user_id,notnull" json:"user_id"`
	RoleID uuid.UUID `bun:"role_id,notnull" json:"role_id"`

	// Current expiration date - gets extended by purchases
	// null only for permanent admin grants
	AutoExpiresAt *time.Time `bun:"auto_expires_at,nullzero" json:"auto_expires_at"`

	GrantedAt time.Time `bun:"granted_at,notnull,default:current_timestamp" json:"granted_at"` // First time this role was granted
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Relationships
	Role       *Role                     `bun:"rel:belongs-to,join:role_id=id" json:"role,omitempty"`
	Payments   []*Payment                `bun:"rel:has-many,join:id=user_role_grant_id" json:"payments,omitempty"`
	Extensions []*UserRoleGrantExtension `bun:"rel:has-many,join:id=user_role_grant_id" json:"extensions,omitempty"`
}

// ExtensionKind distinguishes admin vs grace adjustments
type ExtensionKind string

const (
	ExtensionKindAdmin ExtensionKind = "admin"
	ExtensionKindGrace ExtensionKind = "grace"
)

// UserRoleGrantExtension tracks non-payment adjustments to a role grant (admin or grace)
// This serves the same function as the Payments table, but for manual extensions
type UserRoleGrantExtension struct {
	bun.BaseModel `bun:"table:user_role_grant_extensions,alias:urge"`

	ID   uuid.UUID     `bun:"id,pk,type:uuid" json:"id"`
	Kind ExtensionKind `bun:"kind,notnull" json:"kind"`

	SubscriptionID *uuid.UUID `bun:"subscription_id,type:uuid,nullzero" json:"subscription_id,omitempty"`

	UserRoleGrantID uuid.UUID `bun:"user_role_grant_id,notnull" json:"user_role_grant_id"`
	ExtensionDays   int       `bun:"extension_days,notnull,default:0" json:"extension_days"`

	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
}
