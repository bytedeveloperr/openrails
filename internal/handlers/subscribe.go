package handlers

import (
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/middleware"
)

func Subscribe(r *Request) {
    req := new(SubscribeRequest)
    if err := r.Bind(req); err != nil {
        log.WithError(err).Error("Failed to bind subscribe request")
        r.ErrorJSON(http.StatusBadRequest, "Invalid request")
        return
    }

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	// Idempotency for Mobius tokenized subscribe
	// var userID *uuid.UUID
	// if userCtx.User != nil {
	// uid := userCtx.User.ID
	// userID = &uid
	// }
	// if req.Data.Processor == "mobius" && req.Data.PaymentToken != "" {
	// 	idems := services.NewIdempotencyService(r.State.DB)
	// 	prev, exists, err := idems.Begin(r.Request.Context(), "subscribe.add", req.Data.PaymentToken, userID)
	// 	if err != nil {
	// 		log.WithError(err).Warn("idempotency begin failed; proceeding without cache")
	// 	} else if exists {
	// 		// Return previous result (raw JSON)
	// 		r.Inner().Data(200, "application/json", json.RawMessage(prev.ResultJSON))
	// 		return
	// 	}
	// 	// proceed and on success, complete
	// 	res, err := r.State.SubscriptionService.Subscribe(r.Request.Context(), &req.Data, userCtx.User)
	// 	if err != nil {
	// 		log.WithError(err).Error("failed to subscribe")
	// 		r.ErrorJSON(500, "Internal server error")
	// 		return
	// 	}
	// 	_ = idems.Complete(r.Request.Context(), "subscribe.add", req.Data.PaymentToken, prev.ResultJSON)
	// 	r.SuccessJSON(res)
	// 	return
	// }

	res, err := r.State.SubscriptionService.Subscribe(r.Request.Context(), &req.Data, userCtx.User)
	if err != nil {
		log.WithError(err).Error("failed to subscribe")
		r.ErrorJSON(500, "Internal server error")
		return
	}

	r.SuccessJSON(res)
}
