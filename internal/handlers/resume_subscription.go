package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	riverjobs "github.com/doujins-org/doujins-billing/internal/river"
	"github.com/doujins-org/doujins-billing/pkg/api"
	"github.com/riverqueue/river"
)

// ResumeSubscription resumes a cancelled Stripe subscription (cancel_at_period_end=false).
// POST /v1/me/subscriptions/:id/resume
func ResumeSubscription(r *Request) {
	cl, ok := authgin.ClaimsFromGin(r.GinCtx)
	if !ok || cl.UserID == "" {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	// Parse subscription ID from path
	subscriptionIDStr := r.GinCtx.Param("id")
	if subscriptionIDStr == "" {
		r.ErrorJSON(http.StatusBadRequest, "subscription ID required")
		return
	}

	subscriptionID, err := api.ParseSubscriptionID(subscriptionIDStr)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid subscription ID format")
		return
	}

	if r.State == nil || r.State.DB == nil {
		r.ErrorJSON(http.StatusInternalServerError, "database unavailable")
		return
	}
	if r.State.RiverProducer == nil {
		r.ErrorJSON(http.StatusInternalServerError, "job queue unavailable")
		return
	}
	if r.State.SubscriptionService == nil {
		r.ErrorJSON(http.StatusInternalServerError, "subscription service unavailable")
		return
	}

	// Get subscription by ID and verify ownership
	sub, err := r.State.SubscriptionService.GetByID(r.Request.Context(), subscriptionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			r.ErrorJSON(http.StatusNotFound, "subscription not found")
			return
		}
		r.ErrorJSON(http.StatusInternalServerError, "failed to load subscription")
		return
	}

	// Verify ownership
	if sub.UserID != cl.UserID {
		r.ErrorJSON(http.StatusNotFound, "subscription not found")
		return
	}

	if sub.Processor != models.ProcessorStripe {
		r.ErrorJSON(http.StatusBadRequest, "resume unsupported for processor")
		return
	}

	if sub.Status != models.StatusCancelled {
		r.ErrorJSON(http.StatusBadRequest, "subscription is not cancelled")
		return
	}

	if _, err := r.State.RiverProducer.Insert(r.Request.Context(), riverjobs.ResumeSubscriptionArgs{
		UserID:         cl.UserID,
		SubscriptionID: subscriptionID,
	}, &river.InsertOpts{Queue: riverjobs.QueueBilling}); err != nil {
		r.ErrorJSON(http.StatusInternalServerError, "failed to enqueue resume")
		return
	}

	r.GinCtx.JSON(http.StatusAccepted, map[string]any{"status": "queued"})
}
