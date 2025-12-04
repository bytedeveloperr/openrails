package handlers

import (
	"net/http"

	"github.com/doujins-org/doujins-billing/pkg/api"
)

// GetPrices retrieves all available prices
func GetPrices(r *Request) {
	prices, err := r.State.PriceService.GetAllActive(r.Request.Context())
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
