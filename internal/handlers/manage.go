package handlers

import (
	"net/http"
	"time"

	"github.com/doujins-org/doujins-billing/pkg/api"
)

// ExtendSubscriptionRequest is the request body for ExtendSubscription handler
type ExtendSubscriptionRequest struct {
	SubscriptionID string        `json:"subscription_id" binding:"required"`
	Duration       time.Duration `json:"duration" binding:"required"`
}

func ExtendSubscription(r *Request) {
	var data ExtendSubscriptionRequest
	if !r.BindJSON(&data) {
		return
	}

	subID, err := api.ParseSubscriptionID(data.SubscriptionID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid subscription id")
		return
	}

	if err := r.State.AdminSubscriptionService.ExtendSubscriptionByDuration(r.Request.Context(), subID, data.Duration); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription extended successfully")
}
