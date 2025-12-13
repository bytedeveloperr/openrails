package handlers

import (
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/doujins-org/doujins-billing/internal/processors"
	"github.com/doujins-org/doujins-billing/internal/services"
)

func Subscribe(r *Request) {
	var req SubscribeRequest
	if !r.BindJSON(&req) {
		return
	}

	// Infer processor from route path if not set in body
	// Routes: /subscriptions/mobius, /subscriptions/ccbill, /subscriptions/solana
	if req.Processor == "" {
		// Extract processor from path (e.g., /subscriptions/mobius -> mobius)
		path := r.Request.URL.Path
		parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
		if len(parts) > 0 {
			processor := parts[len(parts)-1]
			// Accept NMI-backed processors, ccbill, and solana
			if processors.IsNMIBacked(processor) || processor == "ccbill" || processor == "solana" {
				req.Processor = processor
			}
		}
	}

	cl, ok := authgin.ClaimsFromGin(r.GinCtx)
	if !ok || cl.UserID == "" {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	user := &services.UserIdentity{
		ID:       cl.UserID,
		Email:    nil,
		Username: cl.Username,
		Roles:    cl.Roles,
	}
	if cl.Email != "" {
		email := cl.Email
		user.Email = &email
	}

	res, err := r.State.SubscriptionService.Subscribe(r.Request.Context(), &req.SubscribeData, user)
	if err != nil {
		log.WithError(err).Error("failed to subscribe")
		r.ErrorJSON(500, "Internal server error")
		return
	}

	r.SuccessJSON(res)
}
