package handlers

import (
	"net/http"
	"strconv"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/query"
)

// GetUserPayments retrieves the user's one-off payments
func GetUserPayments(r *Request) {
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

	paymentType := r.Request.URL.Query().Get("type")

	// Build query options
	queryOpts := &query.QueryOptions[services.GetPaymentsFilters]{
		Limit:   limit,
		Offset:  offset,
		Filters: services.GetPaymentsFilters{},
	}

	if paymentType != "" {
		queryOpts.Filters.Processor = paymentType
	}

	payments, _, err := r.State.UserSubscriptionService.GetUserPayments(
		r.Request.Context(),
		cl.UserID,
		queryOpts,
	)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	r.SuccessJSONPaginated(payments, queryOpts.TotalItems, limit, offset)
}
