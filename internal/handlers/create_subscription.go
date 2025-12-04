package handlers

import (
	"net/http"

	"github.com/doujins-org/doujins-billing/pkg/api"
)

// CreateSubscriptionRequest mirrors Stripe's subscription creation request
type CreateSubscriptionRequest struct {
	Items []struct {
		PriceID  string `json:"price"`    // The price ID
		Quantity int    `json:"quantity"` // Default to 1
	} `json:"items"`
	PaymentMethodID string `json:"default_payment_method"` // Payment Method ID or token
	// Other fields like metadata, customer (inferred from auth)
}

// CreateSubscription handles POST /v1/subscriptions
func CreateSubscription(r *Request) {
	user := r.GetUser()
	if user == nil {
		r.ErrorJSON(http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreateSubscriptionRequest
	if !r.BindJSON(&req) {
		return
	}

	// For now, return a dummy response. Actual logic will be moved here.
	now := r.Clock.Now()
	r.SuccessJSON(api.SubscriptionObject{
		ID:                 "sub_dummy_id",
		Object:             "subscription",
		Status:             "pending",
		Customer:           user.ID,
		StartDate:          api.ToUnix(now),
		CurrentPeriodStart: api.ToUnix(now),
		CurrentPeriodEnd:   api.ToUnix(now.AddDate(0, 1, 0)), // 1 month from now
		CancelAtPeriodEnd:  false,
	})
}
