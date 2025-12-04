package handlers

import (
	"errors"
	"net/http"

	"github.com/doujins-org/doujins-billing/internal/services"
)

func UpdateStatus(r *Request) {
	var data services.UpdateSubscriptionStatusParams
	if !r.BindJSON(&data) {
		return
	}

	// Use new Wave 18 ManageSubscriptionService constructor
	service := services.NewManageSubscriptionService(
		r.State.SubscriptionService,
		r.State.NotificationService,
	)

	if err := service.UpdateStatus(r.Request.Context(), &data); err != nil {
		if errors.Is(err, services.ErrInvalidSubscriptionID) {
			r.ErrorJSON(http.StatusBadRequest, "invalid subscription id")
			return
		}
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription status updated")
}

func ExtendSubscription(r *Request) {
	var data services.ExtendSubscriptionParams
	if !r.BindJSON(&data) {
		return
	}

	// Use new Wave 18 ManageSubscriptionService constructor
	service := services.NewManageSubscriptionService(
		r.State.SubscriptionService,
		r.State.NotificationService,
	)

	if err := service.ExtendSubscription(r.Request.Context(), &data); err != nil {
		if errors.Is(err, services.ErrInvalidSubscriptionID) {
			r.ErrorJSON(http.StatusBadRequest, "invalid subscription id")
			return
		}
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription extended successfully")
}
