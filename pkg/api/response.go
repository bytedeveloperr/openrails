package api

import (
	"time"
)

// ListResponse is a generic list response wrapper matching Stripe's pattern
type ListResponse[T any] struct {
	Object  string `json:"object"`   // Always "list"
	URL     string `json:"url"`      // The URL where this list was retrieved
	HasMore bool   `json:"has_more"` // Whether there are more items
	Data    []T    `json:"data"`     // The list of items
}

// ProductObject represents a product resource
type ProductObject struct {
	ID          string         `json:"id"`
	Object      string         `json:"object"` // Always "product"
	Active      bool           `json:"active"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Created     int64          `json:"created"`
	Updated     int64          `json:"updated"`
	Prices      []PriceObject  `json:"prices,omitempty"` // Expanded prices
}

// PriceObject represents a price resource
type PriceObject struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"` // Always "price"
	Active            bool           `json:"active"`
	Currency          string         `json:"currency"`
	UnitAmount        int64          `json:"unit_amount"` // In cents
	UnitAmountDecimal string         `json:"unit_amount_decimal,omitempty"`
	Product           string         `json:"product"` // Product ID
	BillingScheme     string         `json:"billing_scheme"`
	Recurring         *RecurringInfo `json:"recurring,omitempty"`
	Type              string         `json:"type"` // "recurring" or "one_time"
	Created           int64          `json:"created"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

// RecurringInfo describes the recurring nature of a price
type RecurringInfo struct {
	Interval      string `json:"interval"`       // "day", "week", "month", "year"
	IntervalCount int    `json:"interval_count"` // Number of intervals
}

// SubscriptionObject represents a subscription resource
type SubscriptionObject struct {
	ID                 string                               `json:"id"`
	Object             string                               `json:"object"` // Always "subscription"
	Status             string                               `json:"status"`
	Customer           string                               `json:"customer"`
	Items              ListResponse[SubscriptionItemObject] `json:"items"`
	StartDate          int64                                `json:"start_date"`
	CurrentPeriodStart int64                                `json:"current_period_start"`
	CurrentPeriodEnd   int64                                `json:"current_period_end"`
	CanceledAt         *int64                               `json:"canceled_at,omitempty"`
	EndedAt            *int64                               `json:"ended_at,omitempty"`
	CancelAtPeriodEnd  bool                                 `json:"cancel_at_period_end"`
	Metadata           map[string]any                       `json:"metadata,omitempty"`
	LatestInvoice      *InvoiceObject                       `json:"latest_invoice,omitempty"`
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
	Status       string            `json:"status"` // "succeeded", "requires_action", etc.
	ClientSecret string            `json:"client_secret,omitempty"`
	NextAction   *NextActionObject `json:"next_action,omitempty"`
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
