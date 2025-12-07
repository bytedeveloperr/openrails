package riverjobs

import (
	"context"
	"errors"
	"sync"

	"github.com/doujins-org/doujins-billing/internal/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// Re-export types from jobs package for convenience
type (
	FulfillmentType     = jobs.FulfillmentType
	FulfillPaymentArgs  = jobs.FulfillPaymentArgs
	FulfillmentEnqueuer = jobs.FulfillmentEnqueuer
)

// Re-export constants
const (
	FulfillmentGrantEntitlements  = jobs.FulfillmentGrantEntitlements
	FulfillmentRenewMembership    = jobs.FulfillmentRenewMembership
	FulfillmentCreateSubscription = jobs.FulfillmentCreateSubscription
	KindFulfillPayment            = jobs.KindFulfillPayment
)

// Re-export constructor functions
var (
	NewFulfillPaymentArgsForEntitlements = jobs.NewFulfillPaymentArgsForEntitlements
	NewFulfillPaymentArgsForRenewal      = jobs.NewFulfillPaymentArgsForRenewal
)

// RiverFulfillmentEnqueuer implements FulfillmentEnqueuer using a River client.
type RiverFulfillmentEnqueuer struct {
	client *river.Client[pgx.Tx]
}

// NewRiverFulfillmentEnqueuer creates a new RiverFulfillmentEnqueuer.
func NewRiverFulfillmentEnqueuer(client *river.Client[pgx.Tx]) *RiverFulfillmentEnqueuer {
	return &RiverFulfillmentEnqueuer{client: client}
}

// EnqueueFulfillment enqueues a fulfillment job via River.
func (e *RiverFulfillmentEnqueuer) EnqueueFulfillment(ctx context.Context, args jobs.FulfillPaymentArgs) error {
	_, err := e.client.Insert(ctx, args, &river.InsertOpts{
		Queue:       QueueBilling,
		MaxAttempts: 10, // Allow multiple retry attempts
	})
	return err
}

// LazyFulfillmentEnqueuer wraps a FulfillmentEnqueuer that can be set after construction.
// This solves the circular dependency between services and River client initialization.
type LazyFulfillmentEnqueuer struct {
	mu       sync.RWMutex
	enqueuer jobs.FulfillmentEnqueuer
}

// NewLazyFulfillmentEnqueuer creates a new LazyFulfillmentEnqueuer.
func NewLazyFulfillmentEnqueuer() *LazyFulfillmentEnqueuer {
	return &LazyFulfillmentEnqueuer{}
}

// SetEnqueuer sets the underlying enqueuer. Call this after River client is initialized.
func (l *LazyFulfillmentEnqueuer) SetEnqueuer(e jobs.FulfillmentEnqueuer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enqueuer = e
}

// EnqueueFulfillment enqueues a fulfillment job via the underlying enqueuer.
// Returns an error if the enqueuer hasn't been set yet.
func (l *LazyFulfillmentEnqueuer) EnqueueFulfillment(ctx context.Context, args jobs.FulfillPaymentArgs) error {
	l.mu.RLock()
	e := l.enqueuer
	l.mu.RUnlock()

	if e == nil {
		return errors.New("fulfillment enqueuer not initialized (River client not started)")
	}
	return e.EnqueueFulfillment(ctx, args)
}
