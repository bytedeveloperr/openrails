package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// PaymentMethod represents a stored payment method across multiple processors
// This replaces processor-specific payment method tables
type PaymentMethod struct {
	bun.BaseModel `bun:"table:payment_methods,alias:pm"`

	ID        uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	UserID    string    `bun:"user_id,notnull" json:"user_id"`
	Processor Processor `bun:"processor,notnull" json:"processor"` // "mobius", "ccbill", etc.

	// Processor-specific vault/payment method identifiers
	VaultID              string  `bun:"vault_id,notnull" json:"vault_id"`                             // Primary identifier in processor's system
	BillingID            *string `bun:"billing_id,nullzero" json:"billing_id"`                        // Secondary identifier (e.g., subscription ID)
	InitialTransactionID string  `bun:"initial_transaction_id,notnull" json:"initial_transaction_id"` // Transaction that created this vault

	// Payment method status and metadata
	IsActive      bool    `bun:"is_active,notnull,default:true" json:"is_active"` // Can this method be used for rebills?
	LastFour      *string `bun:"last_four,nullzero" json:"last_four"`             // Last 4 digits of card
	CardType      *string `bun:"card_type,nullzero" json:"card_type"`             // "Visa", "MasterCard", etc.
	ExpiryDate    *string `bun:"expiry_date,nullzero" json:"expiry_date"`         // "MM/YY" format
	FailureReason *string `bun:"failure_reason,nullzero" json:"failure_reason"`   // Reason if inactive
	WalletAddress *string `bun:"wallet_address,nullzero" json:"wallet_address"`   // Solana wallet address for crypto payments

	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	// Relationships
	Subscriptions []*Subscription `bun:"rel:has-many,join:id=payment_method_id" json:"subscriptions,omitempty"`
}

// MarkInactive marks the payment method as inactive (e.g., account closed)
func (pm *PaymentMethod) MarkInactive(reason string) {
	pm.IsActive = false
	pm.FailureReason = &reason
	pm.UpdatedAt = time.Now()
}

// GetType returns the payment method type based on processor
func (pm *PaymentMethod) GetType() string {
	switch pm.Processor {
	case "solana":
		return "wallet"
	case "ccbill":
		return "subscription" // CCBill creates payment methods from subscription webhooks
	case "mobius":
		return "card" // Mobius stores tokenized card details
	default:
		return "unknown"
	}
}

// GetDisplayName returns a user-friendly display name for the payment method
func (pm *PaymentMethod) GetDisplayName() string {
	switch pm.Processor {
	case "solana":
		if pm.WalletAddress != nil {
			addr := *pm.WalletAddress
			if len(addr) > 8 {
				return fmt.Sprintf("Solana Wallet (%s...%s)", addr[:4], addr[len(addr)-4:])
			}
		}
		return "Solana Wallet"
	case "ccbill":
		// CCBill payment methods are created from successful subscriptions
		return "CCBill Subscription"
	case "mobius":
		if pm.LastFour != nil && pm.CardType != nil {
			return fmt.Sprintf("%s ****%s", *pm.CardType, *pm.LastFour)
		}
		return "Mobius Card"
	default:
		return string(pm.Processor)
	}
}

// CanDelete checks if the payment method can be deleted based on active subscriptions
func (pm *PaymentMethod) CanDelete(activeSubscriptions int) (bool, string) {
	if activeSubscriptions > 0 {
		return false, "Cannot delete payment method with active subscriptions"
	}
	return true, ""
}
