package handlers

import (
	"net/http"
	"strconv"

	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/query"
)

// GetSubscriptionHistory retrieves the user's subscription history
func GetSubscriptionHistory(r *Request) {
	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
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

	if status != "" {
		queryOpts.Filters.Status = status
	}

	subscriptions, _, err := r.State.UserSubscriptionService.GetUserSubscriptionHistory(
		r.Request.Context(),
		userCtx.User.ID,
		queryOpts,
	)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONPaginated(subscriptions, queryOpts.TotalItems, limit, offset)
}
