package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// MobiusPaymentMethod represents a stored payment method in Mobius
// This replaces the CustomerVault table and is Mobius-specific
type MobiusPaymentMethod struct {
	bun.BaseModel `bun:"table:mobius_payment_methods,alias:mpm"`

	ID                   uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	UserID               uuid.UUID `bun:"user_id,notnull" json:"user_id"`
	VaultID              string    `bun:"vault_id,notnull" json:"vault_id"`                             // Mobius vault ID
	BillingID            *string   `bun:"billing_id,nullzero" json:"billing_id"`                        // Ientifier In Mobius' system for a recurring subscription / billing arrangement
	InitialTransactionID string    `bun:"initial_transaction_id,notnull" json:"initial_transaction_id"` // Transaction ID of the initial transaction that created this vault
	IsActive             bool      `bun:"is_active,notnull,default:true" json:"is_active"`              // Active or inactive (cannot rebill on inactive)
	CreatedAt            time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt            time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`
}
