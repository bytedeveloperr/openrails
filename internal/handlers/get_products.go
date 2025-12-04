package handlers

import (
	"net/http"

	"github.com/doujins-org/doujins-billing/internal/middleware"
)

// GetProducts retrieves all available products and prices for subscription.
// By default, only active products/prices are returned.
// Admins can pass ?include_inactive=true to see all products/prices.
func GetProducts(r *Request) {
	includeInactive := false

	// Check if admin is requesting inactive items
	if r.Query("include_inactive") == "true" {
		userCtx := middleware.GetUserContext(r.GinCtx)
		if userCtx != nil && userCtx.HasRole("admin") {
			includeInactive = true
		}
		// Non-admins passing include_inactive=true are silently ignored
	}

	products, err := r.State.PublicSubscriptionService.GetProducts(r.Request.Context(), includeInactive)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	response := NewGetProductsResponse(products)
	r.SuccessJSON(response)
}
