package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
)

// GetSubscription retrieves the current user's subscription with enriched data
func GetSubscription(r *Request) {
	cl, ok := authgin.ClaimsFromGin(r.GinCtx)
	if !ok || cl.UserID == "" {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	// Get subscription with enriched data using new Wave 18 service
	subscription, err := r.State.UserSubscriptionService.GetUserSubscription(r.Request.Context(), cl.UserID)
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
