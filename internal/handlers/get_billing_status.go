package handlers

import (
	"net/http"
	"time"
)

type BillingStatusResponse struct {
	Subscription  any        `json:"subscription,omitempty"`
	NextRenewalAt *time.Time `json:"next_renewal_at,omitempty"`
	Entitlements  any        `json:"entitlements,omitempty"`
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
	if r.State.UserSubscriptionService != nil {
		resp, err := r.State.UserSubscriptionService.GetUserSubscription(r.Request.Context(), user.ID)
		if err == nil && resp != nil {
			sub = resp
			if resp.Subscription != nil && resp.Subscription.CurrentPeriodEndsAt != nil {
				next = resp.Subscription.CurrentPeriodEndsAt
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
		Subscription:  sub,
		NextRenewalAt: next,
		Entitlements:  ents,
	})
}
