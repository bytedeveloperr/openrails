package handlers

import (
	"net/http"

	"github.com/google/uuid"
)

// AdminCancelSubscriptionRequest is the request body for admin cancel
type AdminCancelSubscriptionRequest struct {
	Reason string `json:"reason"`
}

// AdminCancelSubscription cancels a subscription by subscription ID (admin)
func AdminCancelSubscription(r *Request) {
	idStr := r.GinCtx.Param("id")
	subscriptionID, err := uuid.Parse(idStr)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid subscription ID")
		return
	}

	req := new(AdminCancelSubscriptionRequest)
	if !r.BindJSON(req) {
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

// AdminCancelUserSubscription cancels a subscription by user ID (admin)
func AdminCancelUserSubscription(r *Request) {
	userID := r.GinCtx.Param("user_id")
	if userID == "" {
		r.ErrorJSON(http.StatusBadRequest, "user_id is required")
		return
	}

	req := new(AdminCancelSubscriptionRequest)
	if !r.BindJSON(req) {
		return
	}

	if err := r.State.AdminSubscriptionService.CancelUserSubscription(
		r.Request.Context(),
		userID,
		req.Reason,
	); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription cancelled successfully")
}
