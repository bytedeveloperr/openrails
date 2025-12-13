package handlers

import (
	"net/http"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/doujins-org/doujins-billing/pkg/api"
	"github.com/doujins-org/ginapi/response"
)

// GetProducts retrieves products and prices for subscription.
// Follows Stripe's API pattern: https://docs.stripe.com/api/products/list
//
// Query params:
//   - active: Only return active (true) or inactive (false) products. Default: true.
//     Non-admins can only see active=true; any other value is silently ignored.
//   - limit: Maximum items to return (default: 20, max: 100)
//   - offset: Number of items to skip (default: 0)
func GetProducts(r *Request) {
	req := new(GetProductsRequest)
	req.SetDefaults()
	if !r.BindQuery(req.Query()) {
		return
	}

	// Determine whether to include inactive products
	includeInactive := false
	if req.Active != nil && !*req.Active {
		// Only admins can view inactive products
		if cl, ok := authgin.ClaimsFromGin(r.GinCtx); ok && cl.HasRole("admin") {
			includeInactive = true
		}
		// Non-admins requesting active=false are silently shown active products only
	}

	result, err := r.State.PublicSubscriptionService.GetProductsPaginated(
		r.Request.Context(),
		includeInactive,
		req.Limit,
		req.Offset,
	)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to API objects
	productObjects := make([]api.ProductObject, len(result.Products))
	for i, p := range result.Products {
		productObjects[i] = ProductToAPI(p.Product, p.Prices)
	}

	listResp := response.NewList(productObjects, result.TotalItems, req.Limit, req.Offset)
	r.SuccessJSON(listResp)
}
