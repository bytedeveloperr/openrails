package handlers

import (
	"net/http"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/pkg/api"
)

// GetPrices retrieves all available prices.
// By default, only active prices are returned.
// Admins can pass ?include_inactive=true to see all prices.
func GetPrices(r *Request) {
	includeInactive := false

	// Check if admin is requesting inactive items
	if r.Query("include_inactive") == "true" {
		userCtx := middleware.GetUserContext(r.GinCtx)
		if userCtx != nil && userCtx.HasRole("admin") {
			includeInactive = true
		}
		// Non-admins passing include_inactive=true are silently ignored
	}

	var prices []*models.Price
	var err error

	if includeInactive {
		prices, err = r.State.PriceService.GetAll(r.Request.Context())
	} else {
		prices, err = r.State.PriceService.GetAllActive(r.Request.Context())
	}
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	priceObjects := make([]api.PriceObject, len(prices))
	for i, p := range prices {
		priceObjects[i] = PriceToAPI(p)
	}

	response := api.ListResponse[api.PriceObject]{
		Object:     "list",
		Data:       priceObjects,
		TotalItems: int64(len(priceObjects)),
	}

	r.SuccessJSON(response)
}
