package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ProcessorType represents the payment processor used for a purchase
type ProcessorType string

const (
	ProcessorPayPal ProcessorType = "paypal"
	ProcessorSolana ProcessorType = "solana"
	ProcessorCCBill ProcessorType = "ccbill"
	ProcessorMobius ProcessorType = "mobius"
)

// Purchase represents a payment event (both one-time and subscription payments)
// This is an immutable event log of all payments received
type Payment struct {
	bun.BaseModel `bun:"table:payments,alias:purch"`

	ID      uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	UserID  uuid.UUID `bun:"user_id,notnull" json:"user_id"`
	PriceID uuid.UUID `bun:"price_id,notnull" json:"price_id"`

	// Optional linkage to the subscription that generated this payment
	SubscriptionID *uuid.UUID `bun:"subscription_id,type:uuid,nullzero" json:"subscription_id,omitempty"`

	Processor     ProcessorType `bun:"processor,notnull" json:"processor"`
	TransactionID string        `bun:"transaction_id,notnull" json:"transaction_id"`

	// Payment details
	Amount   float64 `bun:"amount,notnull,type:numeric" json:"amount"`
	Currency string  `bun:"currency,notnull,default:'USD'" json:"currency"`

	// Role extension details (nullable for products that don't grant roles)
	UserRoleGrantID *uuid.UUID `bun:"user_role_grant_id,type:uuid,nullzero" json:"user_role_grant_id"` // Which role grant this extended (nullable for non-role products)
	ExtensionDays   *int       `bun:"extension_days,nullzero" json:"extension_days"`                   // How many days this purchase extended the role

	PurchasedAt time.Time `bun:"purchased_at,notnull,default:current_timestamp" json:"purchased_at"`
	CreatedAt   time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`

	// Relationships
	Price         *Price         `bun:"rel:belongs-to,join:price_id=id" json:"price,omitempty"`
	Subscription  *Subscription  `bun:"rel:belongs-to,join:subscription_id=id" json:"subscription,omitempty"`
	UserRoleGrant *UserRoleGrant `bun:"rel:belongs-to,join:user_role_grant_id=id" json:"user_role_grant,omitempty"`
}
