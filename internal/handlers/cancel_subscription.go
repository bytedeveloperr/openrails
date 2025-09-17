package handlers

import (
	"net/http"

	"github.com/doujins-org/doujins-billing/internal/middleware"
	log "github.com/sirupsen/logrus"
)

// CancelSubscription cancels the current user's subscription
func CancelSubscription(r *Request) {
	req := new(CancelSubscriptionRequest)
	if err := r.Bind(req); err != nil {
		log.WithError(err).Error("Failed to bind cancel subscription request")
		r.ErrorJSON(http.StatusBadRequest, "Invalid request")
		return
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	if err := r.State.UserSubscriptionService.CancelUserSubscription(
		r.Request.Context(),
		userCtx.User.ID,
		req.Feedback,
	); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription cancelled successfully")
}
