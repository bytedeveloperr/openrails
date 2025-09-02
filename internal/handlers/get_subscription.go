package handlers

import (
	"database/sql"
	"errors"
	"net/http"
)

// GetSubscription retrieves the current user's subscription with enriched data
func GetSubscription(r *Request) {
	user := r.GetUser()

	// Get subscription with enriched data using new Wave 18 service
	subscription, err := r.State.UserSubscriptionService.GetUserSubscription(r.Request.Context(), user.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No active subscription found - return null/empty response
			r.SuccessJSONMessage("no active subscription found")
			return
		}
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSON(subscription)
}
