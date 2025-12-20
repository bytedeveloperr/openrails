package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// WebhookEvent tracks incoming webhook events for deduplication and debugging
type WebhookEvent struct {
	bun.BaseModel `bun:"table:billing.webhook_events,alias:wh"`

	ID uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`

	// Event identification
	Processor string  `bun:",notnull" json:"processor"`
	EventID   *string `bun:"" json:"event_id,omitempty"` // Processor's event ID
	EventType string  `bun:",notnull" json:"event_type"`

	// Processing status
	Status      string     `bun:",notnull,default:'pending'" json:"status"` // pending, processed, failed, duplicate
	ProcessedAt *time.Time `bun:",nullzero" json:"processed_at,omitempty"`

	// Request details
	RawPayload string            `bun:",type:text,notnull" json:"raw_payload"`
	Headers    map[string]string `bun:",type:jsonb" json:"headers,omitempty"`
	IPAddress  string            `bun:",notnull" json:"ip_address"`

	// Signature verification
	SignatureValid *bool   `bun:"" json:"signature_valid,omitempty"`
	Signature      *string `bun:"" json:"signature,omitempty"`

	// Processing results
	ProcessingResult map[string]interface{} `bun:",type:jsonb" json:"processing_result,omitempty"`
	ErrorMessage     *string                `bun:"" json:"error_message,omitempty"`

	// Reference to related entities
	SubscriptionID *uuid.UUID `bun:",type:uuid" json:"subscription_id,omitempty"`
	UserID         *string    `bun:"" json:"user_id,omitempty"`

	// Retry tracking
	ProcessingAttempts int        `bun:",default:0" json:"processing_attempts"`
	LastAttemptAt      *time.Time `bun:",nullzero" json:"last_attempt_at,omitempty"`
	NextAttemptAt      *time.Time `bun:",nullzero" json:"next_attempt_at,omitempty"`

	// Timestamps
	ReceivedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"received_at"`
	CreatedAt  time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt  time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

// SolanaTransaction tracks Solana payment transactions
type SolanaTransaction struct {
	bun.BaseModel `bun:"table:billing.solana_transactions,alias:stx"`

	ID     uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	UserID *string   `bun:"" json:"user_id,omitempty"`

	// Transaction details
	Signature *string `bun:"" json:"signature,omitempty"` // Solana transaction signature
	Status    string  `bun:",notnull" json:"status"`      // pending, confirmed, failed

	// Payment details - amount in smallest unit (lamports for SOL, base units for SPL tokens)
	Amount    int64  `bun:",notnull" json:"amount"`
	Token     string `bun:",notnull" json:"token"`      // SOL, USDC, PYUSD
	TokenMint string `bun:",notnull" json:"token_mint"` // Token mint address

	// Addresses
	FromAddress string     `bun:",notnull" json:"from_address"` // Payer address
	ToAddress   string     `bun:",notnull" json:"to_address"`   // Recipient address
	IntentID    *uuid.UUID `bun:"intent_id" json:"intent_id,omitempty"`

	// Product reference
	ProductID *uuid.UUID `bun:",type:uuid" json:"product_id,omitempty"`
	PaymentID *uuid.UUID `bun:",type:uuid" json:"payment_id,omitempty"`

	// Blockchain details
	BlockTime      *time.Time `bun:",nullzero" json:"block_time,omitempty"`
	Slot           *int64     `bun:"" json:"slot,omitempty"`
	Confirmations  int        `bun:",default:0" json:"confirmations"`
	TransactionFee *int64     `bun:"" json:"transaction_fee,omitempty"` // Fee in lamports

	// Processing metadata
	ProcessingResult map[string]interface{} `bun:",type:jsonb" json:"processing_result,omitempty"`
	ErrorMessage     *string                `bun:"" json:"error_message,omitempty"`

	// QR code reference (for payments initiated via QR)
	QRCodeID *string `bun:"" json:"qr_code_id,omitempty"`

	// Expiration for pending transactions
	ExpiresAt *time.Time `bun:",nullzero" json:"expires_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

const (
	TableWebhookEvents      = "webhook_events"
	TableSolanaTransactions = "solana_transactions"
)

// DailyMetricsSnapshot represents the cached metrics for a single UTC date.
type DailyMetricsSnapshot struct {
	bun.BaseModel `bun:"table:billing.daily_metrics_snapshots,alias:dms"`

	SnapshotDate        time.Time                        `bun:"snapshot_date,pk,notnull" json:"snapshot_date"`
	Currency            string                           `bun:"currency,notnull" json:"currency"`
	MRRCents            int64                            `bun:"mrr_cents,notnull" json:"mrr_cents"`
	SubscriptionRevenue int64                            `bun:"subscription_revenue_cents,notnull" json:"subscription_revenue_cents"`
	OneTimeRevenue      int64                            `bun:"one_time_revenue_cents,notnull" json:"one_time_revenue_cents"`
	RefundsCents        int64                            `bun:"refunds_cents,notnull" json:"refunds_cents"`
	ChargebacksCents    int64                            `bun:"chargebacks_cents,notnull" json:"chargebacks_cents"`
	NewSubscriptions    int                              `bun:"new_subscriptions,notnull" json:"new_subscriptions"`
	ScheduledStarts     int                              `bun:"scheduled_starts,notnull" json:"scheduled_starts"`
	CancellationsVol    int                              `bun:"cancellations_voluntary,notnull" json:"cancellations_voluntary"`
	CancellationsInv    int                              `bun:"cancellations_involuntary,notnull" json:"cancellations_involuntary"`
	Reactivations       int                              `bun:"reactivations,notnull" json:"reactivations"`
	ActiveCountEnd      int                              `bun:"active_count_end,notnull" json:"active_count_end"`
	PastDueCountEnd     int                              `bun:"past_due_count_end,notnull" json:"past_due_count_end"`
	PendingCountEnd     int                              `bun:"pending_count_end,notnull" json:"pending_count_end"`
	EntitlementsGranted int                              `bun:"entitlements_granted,notnull" json:"entitlements_granted"`
	ProcessorBreakdowns map[string]DailyProcessorMetrics `bun:"processor_breakdowns,type:jsonb,notnull" json:"processor_breakdowns"`
	UpdatedAt           time.Time                        `bun:"updated_at,notnull" json:"updated_at"`
}

// DailyProcessorMetrics captures processor-specific revenue + payment counts for a day.
type DailyProcessorMetrics struct {
	Revenue  DailyProcessorRevenue `json:"revenue"`
	Payments DailyProcessorPayment `json:"payments"`

	ActiveSubscriptions int `json:"active_subscriptions"`
	NewSubscriptions    int `json:"new_subscriptions"`
	Cancellations       int `json:"cancellations"`
}

type DailyProcessorRevenue struct {
	Total        int64 `json:"total"`
	Subscription int64 `json:"subscription"`
	OneTime      int64 `json:"one_time"`
	Refunds      int64 `json:"refunds"`
	Chargebacks  int64 `json:"chargebacks"`
}

type DailyProcessorPayment struct {
	Successful int `json:"successful"`
	Failed     int `json:"failed"`
}
