package handlers

import (
	"context"
	"net/http"

	"github.com/doujins-org/doujins-billing/internal/services"
)

func UpdateStatus(r *Request) {
	var data services.UpdateSubscriptionStatusParams
	if err := r.Bind(&data); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid request body")
		return
	}

	// Use new Wave 18 ManageSubscriptionService constructor
	service := services.NewManageSubscriptionService(
		r.State.SubscriptionService,
		r.State.NotificationQueueService,
	)

	if err := service.UpdateStatus(context.Background(), &data); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription status updated")
}

func ExtendSubscription(r *Request) {
	var data services.ExtendSubscriptionParams
	if err := r.Bind(&data); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid request body")
		return
	}

	// Use new Wave 18 ManageSubscriptionService constructor
	service := services.NewManageSubscriptionService(
		r.State.SubscriptionService,
		r.State.NotificationQueueService,
	)

	if err := service.ExtendSubscription(context.Background(), &data); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription extended successfully")
}
