package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/google/uuid"
)

// UpdateStatusRequest is the request body for UpdateStatus handler
type UpdateStatusRequest struct {
	SubscriptionID string                      `json:"subscription_id" binding:"required"`
	Status         models.SubscriptionStatus   `json:"status" binding:"required"`
	CancelFeedback string                      `json:"cancel_feedback"`
	CancelType     models.CancelType           `json:"cancel_type"`
}

func UpdateStatus(r *Request) {
	var data UpdateStatusRequest
	if !r.BindJSON(&data) {
		return
	}

	params := &services.UpdateStatusParams{
		SubscriptionID: data.SubscriptionID,
		Status:         data.Status,
		CancelFeedback: data.CancelFeedback,
		CancelType:     data.CancelType,
	}

	if err := r.State.AdminSubscriptionService.UpdateStatus(r.Request.Context(), params); err != nil {
		if strings.Contains(err.Error(), "invalid subscription id") {
			r.ErrorJSON(http.StatusBadRequest, "invalid subscription id")
			return
		}
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription status updated")
}

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

	subID, err := uuid.Parse(data.SubscriptionID)
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
