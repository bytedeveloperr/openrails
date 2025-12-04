package api

import (
	"time"
)

// ListResponse is a generic list response wrapper with full pagination info.
// We use explicit pagination (total_items, page, page_size, total_pages) instead of
// Stripe's has_more pattern because it provides more useful information to the UI.
type ListResponse[T any] struct {
	Object     string `json:"object"`                // Always "list"
	Data       []T    `json:"data"`                  // The list of items
	TotalItems int64  `json:"total_items"`           // Total number of items across all pages
	Page       int    `json:"page,omitempty"`        // Current page number (1-indexed)
	PageSize   int    `json:"page_size,omitempty"`   // Number of items per page
	TotalPages int    `json:"total_pages,omitempty"` // Total number of pages
}

// ProductObject represents a product resource
type ProductObject struct {
	ID          string        `json:"id"`
	Object      string        `json:"object"` // Always "product"
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Active      bool          `json:"active"`
	Created     int64         `json:"created"`
	Updated     int64         `json:"updated"`
	Prices      []PriceObject `json:"prices,omitempty"`
}

// PriceObject represents a price resource
type PriceObject struct {
	ID        string         `json:"id"`
	Object    string         `json:"object"` // Always "price"
	Name      string         `json:"name"`
	Amount    int64          `json:"amount"` // In cents
	Currency  string         `json:"currency"`
	Recurring *RecurringInfo `json:"recurring,omitempty"` // null for one-time purchases
	Product   string         `json:"product"`             // Product ID
	Active    bool           `json:"active"`
	Created   int64          `json:"created"`
}

// RecurringInfo describes the billing interval for recurring prices
type RecurringInfo struct {
	Interval      string `json:"interval"`       // "day", "week", "month", "year"
	IntervalCount int    `json:"interval_count"` // Number of intervals between billings
}

// SubscriptionObject represents a subscription resource
type SubscriptionObject struct {
	ID                 string                   `json:"id"`
	Object             string                   `json:"object"` // Always "subscription"
	Status             string                   `json:"status"`
	Customer           string                   `json:"customer"`
	Items              []SubscriptionItemObject `json:"items"` // Subscription items (typically just one)
	StartDate          int64                    `json:"start_date"`
	CurrentPeriodStart int64                    `json:"current_period_start"`
	CurrentPeriodEnd   int64                    `json:"current_period_end"`
	CanceledAt         *int64                   `json:"canceled_at,omitempty"`
	EndedAt            *int64                   `json:"ended_at,omitempty"`
	CancelAtPeriodEnd  bool                     `json:"cancel_at_period_end"`
	LatestInvoice      *InvoiceObject           `json:"latest_invoice,omitempty"`
}

// SubscriptionItemObject represents an item in a subscription
type SubscriptionItemObject struct {
	ID           string      `json:"id"`
	Object       string      `json:"object"` // Always "subscription_item"
	Price        PriceObject `json:"price"`
	Subscription string      `json:"subscription"`
	Quantity     int         `json:"quantity"`
}

// InvoiceObject represents an invoice (simplified for now)
type InvoiceObject struct {
	ID            string               `json:"id"`
	Object        string               `json:"object"` // Always "invoice"
	Status        string               `json:"status"`
	PaymentIntent *PaymentIntentObject `json:"payment_intent,omitempty"`
}

// PaymentIntentObject represents a payment intent
type PaymentIntentObject struct {
	ID           string            `json:"id"`
	Object       string            `json:"object"` // Always "payment_intent"
	Status       string            `json:"status"` // "pending", "processing", "confirmed", "failed"
	Amount       int64             `json:"amount"` // Amount in cents (USD)
	Currency     string            `json:"currency"`
	ClientSecret string            `json:"client_secret,omitempty"`
	NextAction   *NextActionObject `json:"next_action,omitempty"`
	// Solana-specific fields
	PaymentMethod *PaymentIntentPaymentMethod `json:"payment_method,omitempty"`
	Transaction   *PaymentIntentTransaction   `json:"transaction,omitempty"`
	ExpiresAt     int64                       `json:"expires_at,omitempty"`
	Created       int64                       `json:"created"`
}

// PaymentIntentPaymentMethod describes the payment method for the intent
type PaymentIntentPaymentMethod struct {
	Type        string `json:"type"`                   // "solana"
	Token       string `json:"token,omitempty"`        // Token symbol (SOL, USDC)
	TokenMint   string `json:"token_mint,omitempty"`   // Token mint address
	TokenAmount string `json:"token_amount,omitempty"` // Human-readable token amount
	Wallet      string `json:"wallet,omitempty"`       // Payer wallet address
}

// PaymentIntentTransaction contains transaction details for Solana payments
type PaymentIntentTransaction struct {
	// For direct flow: base64-encoded transaction to sign
	Data string `json:"data,omitempty"`
	// For QR flow: Solana Pay URL
	URL string `json:"url,omitempty"`
	// Reference key for looking up on-chain (QR flow)
	Reference string `json:"reference,omitempty"`
	// Transaction signature after confirmation
	Signature string `json:"signature,omitempty"`
}

// NextActionObject describes the next action the user must take
type NextActionObject struct {
	Type          string               `json:"type"`
	RedirectToURL *RedirectToURLObject `json:"redirect_to_url,omitempty"`
}

// RedirectToURLObject contains the URL to redirect to
type RedirectToURLObject struct {
	URL       string `json:"url"`
	ReturnURL string `json:"return_url,omitempty"`
}

// PaymentObject represents a payment resource
type PaymentObject struct {
	ID              string              `json:"id"`
	Object          string              `json:"object"` // Always "payment"
	Amount          int64               `json:"amount"` // Amount in cents (positive for payments, negative for refunds)
	AmountRefunded  int64               `json:"amount_refunded"`
	Currency        string              `json:"currency"`
	Customer        string              `json:"customer"`                      // User ID with cus_ prefix
	Subscription    *string             `json:"subscription,omitempty"`        // Subscription ID if linked
	Processor       string              `json:"processor"`                     // mobius, ccbill, solana
	TransactionID   string              `json:"transaction_id"`                // Processor's transaction identifier
	Refunded        bool                `json:"refunded"`                      // True if fully refunded
	Refunds         *PaymentRefundsList `json:"refunds,omitempty"`             // List of refunds (for single payment view)
	Created         int64               `json:"created"`                       // Unix epoch seconds
	Price           *PriceObject        `json:"price,omitempty"`               // Expanded price object
	SubscriptionObj *SubscriptionObject `json:"subscription_object,omitempty"` // Expanded subscription (for detail view)
}

// PaymentRefundsList contains refund entries for a payment
type PaymentRefundsList struct {
	Object string          `json:"object"` // Always "list"
	Data   []PaymentObject `json:"data"`
}

// Helper to convert time.Time to unix epoch
func ToUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// Helper to convert pointer to time.Time to pointer to unix epoch
func ToUnixPtr(t *time.Time) *int64 {
	if t == nil || t.IsZero() {
		return nil
	}
	ts := t.Unix()
	return &ts
}
