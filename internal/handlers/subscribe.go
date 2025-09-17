package handlers

import (
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/middleware"
)

func Subscribe(r *Request) {
	var req SubscribeRequest
	if err := r.Bind(&req); err != nil {
		log.WithError(err).Error("Failed to bind subscribe request")
		r.ErrorJSON(http.StatusBadRequest, "Invalid request")
		return
	}

	if req.Processor == "" {
		req.Processor = r.Param("processor")
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	res, err := r.State.SubscriptionService.Subscribe(r.Request.Context(), &req.SubscribeData, userCtx.User)
	if err != nil {
		log.WithError(err).Error("failed to subscribe")
		r.ErrorJSON(500, "Internal server error")
		return
	}

	r.SuccessJSON(res)
}
