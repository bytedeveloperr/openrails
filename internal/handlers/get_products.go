package handlers

import (
	"net/http"
)

// GetProducts retrieves all available products and prices for subscription
func GetProducts(r *Request) {
	products, err := r.State.PublicSubscriptionService.GetAvailableProducts(r.Request.Context())
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	response := NewGetProductsResponse(products)
	r.SuccessJSON(response)
}
