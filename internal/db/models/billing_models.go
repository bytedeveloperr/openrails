package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// BillingEvent tracks all billing-related events for analytics and auditing
type BillingEvent struct {
	bun.BaseModel `bun:"table:billing_events"`

	ID     uuid.UUID  `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	UserID *uuid.UUID `bun:",type:uuid" json:"user_id,omitempty"`

	// Event classification
	EventType string `bun:",notnull" json:"event_type"` // subscription_created, payment_processed, etc.
	Processor string `bun:",notnull" json:"processor"`  // mobius, ccbill, solana
	Status    string `bun:",notnull" json:"status"`     // success, failed, pending

	// Reference IDs
	SubscriptionID   *uuid.UUID `bun:",type:uuid" json:"subscription_id,omitempty"`
	TransactionID    *string    `bun:"" json:"transaction_id,omitempty"`     // Processor transaction ID
	ProcessorEventID *string    `bun:"" json:"processor_event_id,omitempty"` // Webhook event ID

	// Financial details
	Amount   *float64 `bun:",type:decimal(10,2)" json:"amount,omitempty"`
	Currency *string  `bun:"" json:"currency,omitempty"`

	// Event metadata (JSON for flexibility)
	Metadata map[string]interface{} `bun:",type:jsonb" json:"metadata,omitempty"`

	// Error information
	ErrorMessage *string `bun:"" json:"error_message,omitempty"`
	ErrorCode    *string `bun:"" json:"error_code,omitempty"`

	// Request tracking
	RequestID *string `bun:"" json:"request_id,omitempty"`
	IPAddress *string `bun:"" json:"ip_address,omitempty"`
	UserAgent *string `bun:"" json:"user_agent,omitempty"`

	// Timestamps
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

// ProcessorSubscription tracks processor-specific subscription details
type ProcessorSubscription struct {
	bun.BaseModel `bun:"table:processor_subscriptions"`

	ID             uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	SubscriptionID uuid.UUID `bun:",type:uuid,notnull" json:"subscription_id"`

	// Processor details
	Processor           string  `bun:",notnull" json:"processor"`                 // mobius, ccbill
	ProcessorSubID      string  `bun:",notnull" json:"processor_subscription_id"` // Processor's subscription ID
	ProcessorCustomerID *string `bun:"" json:"processor_customer_id,omitempty"`   // Customer/vault ID

	// Current state
	Status string `bun:",notnull" json:"status"` // active, cancelled, expired, suspended

	// Billing details
	Amount       float64 `bun:",type:decimal(10,2),notnull" json:"amount"`
	Currency     string  `bun:",notnull" json:"currency"`
	BillingCycle string  `bun:",notnull" json:"billing_cycle"` // monthly, yearly

	// Important dates
	StartedAt    time.Time  `bun:",nullzero,notnull" json:"started_at"`
	LastBilledAt *time.Time `bun:",nullzero" json:"last_billed_at,omitempty"`
	NextBillAt   *time.Time `bun:",nullzero" json:"next_bill_at,omitempty"`
	ExpiresAt    *time.Time `bun:",nullzero" json:"expires_at,omitempty"`
	CancelledAt  *time.Time `bun:",nullzero" json:"cancelled_at,omitempty"`

	// Failure tracking
	FailedAttempts    int        `bun:",default:0" json:"failed_attempts"`
	LastFailedAt      *time.Time `bun:",nullzero" json:"last_failed_at,omitempty"`
	LastFailureReason *string    `bun:"" json:"last_failure_reason,omitempty"`

	// Metadata for processor-specific fields
	ProcessorMetadata map[string]interface{} `bun:",type:jsonb" json:"processor_metadata,omitempty"`

	// Timestamps
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

// PaymentAttempt tracks individual payment attempts (for retry logic)
type PaymentAttempt struct {
	bun.BaseModel `bun:"table:payment_attempts"`

	ID                      uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	ProcessorSubscriptionID uuid.UUID `bun:",type:uuid,notnull" json:"processor_subscription_id"`

	// Attempt details
	AttemptNumber int    `bun:",notnull" json:"attempt_number"`
	Processor     string `bun:",notnull" json:"processor"`

	// Transaction details
	Amount            float64                `bun:",type:decimal(10,2),notnull" json:"amount"`
	Currency          string                 `bun:",notnull" json:"currency"`
	TransactionID     *string                `bun:"" json:"transaction_id,omitempty"`
	ProcessorResponse map[string]interface{} `bun:",type:jsonb" json:"processor_response,omitempty"`

	// Result
	Status       string  `bun:",notnull" json:"status"` // success, failed, pending
	ErrorMessage *string `bun:"" json:"error_message,omitempty"`
	ErrorCode    *string `bun:"" json:"error_code,omitempty"`

	// Scheduling
	ScheduledAt time.Time  `bun:",nullzero,notnull" json:"scheduled_at"`
	AttemptedAt *time.Time `bun:",nullzero" json:"attempted_at,omitempty"`
	CompletedAt *time.Time `bun:",nullzero" json:"completed_at,omitempty"`
	NextRetryAt *time.Time `bun:",nullzero" json:"next_retry_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

// WebhookEvent tracks incoming webhook events for deduplication and debugging
type WebhookEvent struct {
	bun.BaseModel `bun:"table:webhook_events"`

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
	UserID         *uuid.UUID `bun:",type:uuid" json:"user_id,omitempty"`

	// Retry tracking
	ProcessingAttempts int        `bun:",default:0" json:"processing_attempts"`
	LastAttemptAt      *time.Time `bun:",nullzero" json:"last_attempt_at,omitempty"`

	// Timestamps
	ReceivedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"received_at"`
	CreatedAt  time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt  time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

// SolanaTransaction tracks Solana payment transactions
type SolanaTransaction struct {
	bun.BaseModel `bun:",table:solana_transactions"`

	ID     uuid.UUID  `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`
	UserID *uuid.UUID `bun:",type:uuid" json:"user_id,omitempty"`

	// Transaction details
	Signature *string `bun:"" json:"signature,omitempty"` // Solana transaction signature
	Status    string  `bun:",notnull" json:"status"`      // pending, confirmed, failed

	// Payment details
	Amount    float64 `bun:",type:decimal(18,9),notnull" json:"amount"`
	Token     string  `bun:",notnull" json:"token"`      // SOL, USDC, PYUSD
	TokenMint string  `bun:",notnull" json:"token_mint"` // Token mint address

	// Addresses
	FromAddress string `bun:",notnull" json:"from_address"` // Payer address
	ToAddress   string `bun:",notnull" json:"to_address"`   // Recipient address

	// Product reference
	ProductID  *uuid.UUID `bun:",type:uuid" json:"product_id,omitempty"`
	PurchaseID *uuid.UUID `bun:",type:uuid" json:"purchase_id,omitempty"`

	// Blockchain details
	BlockTime      *time.Time `bun:",nullzero" json:"block_time,omitempty"`
	Slot           *int64     `bun:"" json:"slot,omitempty"`
	Confirmations  int        `bun:",default:0" json:"confirmations"`
	TransactionFee *float64   `bun:",type:decimal(18,9)" json:"transaction_fee,omitempty"`

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

// BillingMetric stores aggregated billing metrics for analytics
type BillingMetric struct {
	bun.BaseModel `bun:"table:billing_metrics"`

	ID uuid.UUID `bun:",pk,type:uuid,default:gen_random_uuid()" json:"id"`

	// Metric identification
	MetricType string    `bun:",notnull" json:"metric_type"` // revenue, subscriptions, failures, etc.
	Processor  *string   `bun:"" json:"processor,omitempty"` // mobius, ccbill, solana, or null for aggregated
	Period     string    `bun:",notnull" json:"period"`      // hour, day, week, month
	PeriodDate time.Time `bun:",nullzero,notnull" json:"period_date"`

	// Metric values (use appropriate fields based on metric type)
	Count   *int64   `bun:"" json:"count,omitempty"`                     // For counting events
	Amount  *float64 `bun:",type:decimal(10,2)" json:"amount,omitempty"` // For revenue/amounts
	Average *float64 `bun:",type:decimal(10,2)" json:"average,omitempty"`

	// Additional dimensions
	Currency *string                `bun:"" json:"currency,omitempty"`
	Metadata map[string]interface{} `bun:",type:jsonb" json:"metadata,omitempty"`

	// Timestamps
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

// Table names as constants for consistency
const (
	TableBillingEvents          = "billing_events"
	TableProcessorSubscriptions = "processor_subscriptions"
	TablePaymentAttempts        = "payment_attempts"
	TableWebhookEvents          = "webhook_events"
	TableSolanaTransactions     = "solana_transactions"
	TableBillingMetrics         = "billing_metrics"
)

// Event types for BillingEvent
const (
	EventSubscriptionCreated    = "subscription_created"
	EventSubscriptionCancelled  = "subscription_cancelled"
	EventSubscriptionExpired    = "subscription_expired"
	EventSubscriptionRenewed    = "subscription_renewed"
	EventPaymentProcessed       = "payment_processed"
	EventPaymentFailed          = "payment_failed"
	EventWebhookReceived        = "webhook_received"
	EventWebhookProcessed       = "webhook_processed"
	EventRefundProcessed        = "refund_processed"
	EventChargebackReceived     = "chargeback_received"
	EventSolanaPaymentConfirmed = "solana_payment_confirmed"
)
