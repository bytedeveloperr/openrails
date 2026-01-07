package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type CheckoutSessionMode string

const (
	CheckoutSessionModeOneOff       CheckoutSessionMode = "one_off"
	CheckoutSessionModeSubscription CheckoutSessionMode = "subscription"
)

type CheckoutSessionStatus string

const (
	CheckoutSessionStatusCreated        CheckoutSessionStatus = "created"
	CheckoutSessionStatusRequiresAction CheckoutSessionStatus = "requires_action"
	CheckoutSessionStatusSucceeded      CheckoutSessionStatus = "succeeded"
	CheckoutSessionStatusFailed         CheckoutSessionStatus = "failed"
	CheckoutSessionStatusExpired        CheckoutSessionStatus = "expired"
	CheckoutSessionStatusCanceled       CheckoutSessionStatus = "canceled"
)

type CheckoutSession struct {
	bun.BaseModel `bun:"table:billing.checkout_sessions,alias:cs"`

	ID     uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	UserID string    `bun:"user_id,notnull" json:"user_id"`

	PriceID uuid.UUID           `bun:"price_id,type:uuid,notnull" json:"price_id"`
	Mode    CheckoutSessionMode `bun:"mode,notnull" json:"mode"`

	Processor Processor             `bun:"processor,notnull" json:"processor"`
	Status    CheckoutSessionStatus `bun:"status,notnull" json:"status"`

	Amount   int64  `bun:"amount,notnull" json:"amount"`
	Currency string `bun:"currency,notnull,default:'usd'" json:"currency"`

	ExpiresAt *time.Time `bun:"expires_at,nullzero" json:"expires_at,omitempty"`
	Reference *string    `bun:"reference,nullzero" json:"reference,omitempty"`

	TransactionID  *string    `bun:"transaction_id,nullzero" json:"transaction_id,omitempty"`
	PaymentID      *uuid.UUID `bun:"payment_id,type:uuid,nullzero" json:"payment_id,omitempty"`
	SubscriptionID *uuid.UUID `bun:"subscription_id,type:uuid,nullzero" json:"subscription_id,omitempty"`

	Metadata map[string]string `bun:"metadata,type:jsonb,nullzero" json:"metadata,omitempty"`

	ProcessorFields map[string]any `bun:"processor_fields,type:jsonb,nullzero" json:"processor_fields,omitempty"`
	ProcessorState  map[string]any `bun:"processor_state,type:jsonb,nullzero" json:"processor_state,omitempty"`

	IdempotencyKey *string   `bun:"idempotency_key,nullzero" json:"idempotency_key,omitempty"`
	CreatedAt      time.Time `bun:"created_at,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt      time.Time `bun:"updated_at,notnull,default:current_timestamp" json:"updated_at"`

	Price *Price `bun:"rel:belongs-to,join:price_id=id" json:"price,omitempty"`
}
