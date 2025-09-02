package handlers

import (
	"net/http"

	log "github.com/sirupsen/logrus"
)

// CancelSubscription cancels the current user's subscription
func CancelSubscription(r *Request) {
	var req *CancelSubscriptionRequest
	if err := r.Bind(req); err != nil {
		log.WithError(err).Error("Failed to bind cancel subscription request")
		r.ErrorJSON(http.StatusBadRequest, "Invalid request")
		return
	}

	user := r.GetUser()
	if err := r.State.UserSubscriptionService.CancelUserSubscription(
		r.Request.Context(),
		user.ID,
		req.Feedback,
	); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription cancelled successfully")
}
