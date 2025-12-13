package handlers

import (
	"net/http"
	"strconv"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/query"
)

// GetMySubscriptions retrieves the user's subscriptions with query param filtering
// Query params:
//   - status: filter by status (active, cancelled, past_due, all). Default: non-cancelled
//   - limit: max results (1-100, default 10)
//   - offset: pagination offset (default 0)
func GetMySubscriptions(r *Request) {
	cl, ok := authgin.ClaimsFromGin(r.GinCtx)
	if !ok || cl.UserID == "" {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	// Parse query parameters
	limit, _ := strconv.Atoi(r.Request.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	offset, _ := strconv.Atoi(r.Request.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	status := r.Request.URL.Query().Get("status")

	// Build query options
	queryOpts := &query.QueryOptions[services.GetSubscriptionsFilters]{
		Limit:   limit,
		Offset:  offset,
		Filters: services.GetSubscriptionsFilters{},
	}

	// Handle status filtering
	// Default behavior: show non-cancelled subscriptions (like Stripe)
	// status=all: show everything including cancelled
	// status=active: only active
	// status=cancelled: only cancelled
	if status != "" && status != "all" {
		queryOpts.Filters.Status = status
	}

	subscriptions, _, err := r.State.UserSubscriptionService.GetUserSubscriptionHistory(
		r.Request.Context(),
		cl.UserID,
		queryOpts,
	)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONPaginated(subscriptions, queryOpts.TotalItems, limit, offset)
}
