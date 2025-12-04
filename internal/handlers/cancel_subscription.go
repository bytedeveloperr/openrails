package handlers

import (
	"errors"
	"net/http"

	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/services"
)

// CancelSubscription cancels the current user's subscription
func CancelSubscription(r *Request) {
	req := new(CancelSubscriptionRequest)
	if !r.BindJSON(req) {
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
		// Check if this is a CCBill cancellation error with support URL
		var ccbillErr *services.CCBillCancelError
		if errors.As(err, &ccbillErr) {
			r.GinCtx.JSON(http.StatusUnprocessableEntity, map[string]any{
				"error":       ccbillErr.Message,
				"support_url": ccbillErr.SupportURL,
				"code":        "ccbill_cancel_required",
			})
			return
		}

		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONMessage("subscription cancelled successfully")
}
