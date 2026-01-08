package handlers

import (
	"net/http"

	"github.com/doujins-org/doujins-billing/pkg/api"
)

// AdminCancelSubscriptionRequest is the request body for admin cancel
type AdminCancelSubscriptionRequest struct {
	Reason string `json:"reason"`
}

// AdminCancelSubscription cancels a subscription by subscription ID (admin)
func AdminCancelSubscription(r *Request) {
	subscriptionID, err := api.ParseSubscriptionID(r.GinCtx.Param("id"))
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid subscription ID")
		return
	}

	req := new(AdminCancelSubscriptionRequest)
	if !r.BindJSON(req) {
		r.ErrorJSON(http.StatusBadRequest, "invalid request body")
		return
	}

	if err := r.State.AdminSubscriptionService.CancelSubscription(
		r.Request.Context(),
		subscriptionID,
		req.Reason,
	); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription cancelled successfully")
}
