package handlers

import (
	"net/http"
	"strconv"

	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/query"
)

// GetUserPurchases retrieves the user's one-off payments
func GetUserPurchases(r *Request) {
	user := r.GetUser()

	// Parse query parameters
	limit, _ := strconv.Atoi(r.Request.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	offset, _ := strconv.Atoi(r.Request.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	purchaseType := r.Request.URL.Query().Get("type")

	// Build query options
	queryOpts := &query.QueryOptions[services.GetPaymentsFilters]{
		Limit:   limit,
		Offset:  offset,
		Filters: services.GetPaymentsFilters{},
	}

	if purchaseType != "" {
		queryOpts.Filters.Processor = purchaseType
	}

	purchases, _, err := r.State.UserSubscriptionService.GetUserPurchases(
		r.Request.Context(),
		user.ID,
		queryOpts,
	)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	response := PaginatedResponse{Data: purchases, TotalItems: queryOpts.TotalItems}
	r.SuccessJSON(response)
}
