// Package jobs defines job arguments and interfaces for background job processing.
// This package exists to avoid circular dependencies between services and river.
package jobs

import (
	"context"

	"github.com/google/uuid"
)

// FulfillmentType indicates what post-charge operation needs to be completed
type FulfillmentType string

const (
	// FulfillmentGrantEntitlements - grant entitlements for a one-time purchase
	FulfillmentGrantEntitlements FulfillmentType = "grant_entitlements"

	// FulfillmentRenewMembership - extend subscription period after successful rebill
	FulfillmentRenewMembership FulfillmentType = "renew_membership"

	// FulfillmentCreateSubscription - create subscription record after NMI subscription created
	FulfillmentCreateSubscription FulfillmentType = "create_subscription"
)

// FulfillPaymentArgs contains everything needed to fulfill a payment that
// was successfully charged but failed to complete its post-charge operations.
type FulfillPaymentArgs struct {
	// What type of fulfillment is needed
	FulfillmentType FulfillmentType `json:"fulfillment_type"`

	// Payment details (always present)
	PaymentID     uuid.UUID `json:"payment_id"`
	UserID        string    `json:"user_id"`
	PriceID       uuid.UUID `json:"price_id"`
	TransactionID string    `json:"transaction_id"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`

	// For subscription renewals
	SubscriptionID          *uuid.UUID `json:"subscription_id,omitempty"`
	ProcessorSubscriptionID string     `json:"processor_subscription_id,omitempty"`

	// Original error that caused the failure (for logging/debugging)
	OriginalError string `json:"original_error,omitempty"`
}

// Kind implements river.JobArgs. Required for River to identify the job type.
const KindFulfillPayment = "billing.fulfill_payment"

func (FulfillPaymentArgs) Kind() string { return KindFulfillPayment }

// FulfillmentEnqueuer provides an interface for enqueueing fulfillment jobs.
// This allows services to enqueue jobs without a direct dependency on river.Client.
type FulfillmentEnqueuer interface {
	EnqueueFulfillment(ctx context.Context, args FulfillPaymentArgs) error
}

// NewFulfillPaymentArgsForEntitlements creates args for granting entitlements after a one-time purchase.
func NewFulfillPaymentArgsForEntitlements(
	paymentID uuid.UUID,
	userID string,
	priceID uuid.UUID,
	transactionID string,
	amount int64,
	currency string,
	originalError string,
) FulfillPaymentArgs {
	return FulfillPaymentArgs{
		FulfillmentType: FulfillmentGrantEntitlements,
		PaymentID:       paymentID,
		UserID:          userID,
		PriceID:         priceID,
		TransactionID:   transactionID,
		Amount:          amount,
		Currency:        currency,
		OriginalError:   originalError,
	}
}

// NewFulfillPaymentArgsForRenewal creates args for renewing membership after a successful rebill.
func NewFulfillPaymentArgsForRenewal(
	paymentID uuid.UUID,
	userID string,
	priceID uuid.UUID,
	transactionID string,
	amount int64,
	currency string,
	subscriptionID uuid.UUID,
	processorSubscriptionID string,
	originalError string,
) FulfillPaymentArgs {
	return FulfillPaymentArgs{
		FulfillmentType:         FulfillmentRenewMembership,
		PaymentID:               paymentID,
		UserID:                  userID,
		PriceID:                 priceID,
		TransactionID:           transactionID,
		Amount:                  amount,
		Currency:                currency,
		SubscriptionID:          &subscriptionID,
		ProcessorSubscriptionID: processorSubscriptionID,
		OriginalError:           originalError,
	}
}
