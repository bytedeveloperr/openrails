package handlers

import (
	"net/http"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db/models"
)

type BillingStatusResponse struct {
	HasActiveSubscription bool       `json:"has_active_subscription"`
	Subscription          any        `json:"subscription,omitempty"`
	NextRenewalAt         *time.Time `json:"next_renewal_at,omitempty"`
	Entitlements          any        `json:"entitlements,omitempty"`
}

func GetMyBillingStatus(r *Request) {
	user := r.GetUser()
	if user == nil {
		r.ErrorJSON(http.StatusUnauthorized, "unauthorized")
		return
	}

	// Subscription details
	var sub any
	var next *time.Time
	var hasActive bool
	if r.State.UserSubscriptionService != nil {
		resp, err := r.State.UserSubscriptionService.GetUserSubscription(r.Request.Context(), user.ID)
		if err == nil && resp != nil {
			sub = resp
			if resp.Subscription != nil {
				// Check if subscription is active
				hasActive = resp.Subscription.Status == models.StatusActive
				if resp.Subscription.CurrentPeriodEndsAt != nil {
					next = resp.Subscription.CurrentPeriodEndsAt
				}
			}
		}
	}

	// List entitlements (optional)
	var ents any
	if r.State.EntitlementService != nil {
		list, err := r.State.EntitlementService.ListByUser(r.Request.Context(), user.ID)
		if err != nil {
			r.ErrorJSON(http.StatusInternalServerError, err.Error())
			return
		}
		ents = list
	}

	r.SuccessJSON(BillingStatusResponse{
		HasActiveSubscription: hasActive,
		Subscription:          sub,
		NextRenewalAt:         next,
		Entitlements:          ents,
	})
}
