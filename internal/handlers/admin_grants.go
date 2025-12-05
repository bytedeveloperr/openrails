package handlers

import (
	"net/http"

	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/api"
	"github.com/doujins-org/doujins-billing/pkg/query"
)

// AdminGrantRequest is the request body for creating an admin grant
type AdminGrantRequest struct {
	PriceID      string `json:"price_id" binding:"required"` // Price being granted (determines product & entitlements)
	Reason       string `json:"reason" binding:"required"`   // Reason for grant (comp, contest_winner, etc.)
	DurationDays *int   `json:"duration_days,omitempty"`     // Optional: nil=use spec, 0=indefinite, N=N days

	// Optional payment info (only if money was received)
	Amount        int64  `json:"amount,omitempty"`         // Amount in cents (0 = free comp)
	Currency      string `json:"currency,omitempty"`       // Currency code (defaults to price.Currency)
	TransactionID string `json:"transaction_id,omitempty"` // External reference (PayPal ID, cash receipt, etc.)
}

// AdminGrantResponse is the response body for a successful admin grant
type AdminGrantResponse struct {
	Object              string   `json:"object"`
	AdminGrantID        string   `json:"admin_grant_id"`
	PaymentID           *string  `json:"payment_id,omitempty"`
	EntitlementsGranted []string `json:"entitlements_granted"`
}

// CreateAdminGrant grants a product to a user (admin action)
// POST /v1/admin/users/:user_id/grants
func CreateAdminGrant(r *Request) {
	var path AdminUserEntitlementsPath
	if err := r.Inner().ShouldBindUri(&path); err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	var req AdminGrantRequest
	if !r.BindJSON(&req) {
		return
	}

	svc := r.State.AdminGrantService
	if svc == nil {
		r.ErrorJSON(http.StatusInternalServerError, "admin grant service unavailable")
		return
	}

	// Parse price ID (strip prefix if present)
	priceID, _, err := api.TryParseID(req.PriceID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid price_id format")
		return
	}

	// Get admin user ID from JWT
	adminUser := r.GetUser()
	if adminUser == nil || adminUser.ID == "" {
		r.ErrorJSON(http.StatusUnauthorized, "admin user ID not found in token")
		return
	}
	adminUserID := adminUser.ID

	// Build grant request
	grantReq := &services.AdminGrantRequest{
		UserID:        path.UserID,
		PriceID:       priceID,
		GrantedBy:     adminUserID,
		Reason:        req.Reason,
		DurationDays:  req.DurationDays,
		Amount:        req.Amount,
		Currency:      req.Currency,
		TransactionID: req.TransactionID,
	}

	result, err := svc.Grant(r.Request.Context(), grantReq)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	// Build response
	resp := AdminGrantResponse{
		Object:              "admin_grant",
		AdminGrantID:        api.FormatAdminGrantID(result.AdminGrantID),
		EntitlementsGranted: result.EntitlementsGranted,
	}
	if result.PaymentID != nil {
		paymentIDStr := api.FormatPaymentID(*result.PaymentID)
		resp.PaymentID = &paymentIDStr
	}

	r.GinCtx.JSON(http.StatusCreated, resp)
}

// GetAdminGrant retrieves an admin grant by ID (admin action)
// GET /v1/admin/grants/:id
func GetAdminGrant(r *Request) {
	grantIDStr := r.GinCtx.Param("id")

	// Parse grant ID (strip prefix if present)
	grantID, _, err := api.TryParseID(grantIDStr)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid grant ID format")
		return
	}

	svc := r.State.AdminGrantService
	if svc == nil {
		r.ErrorJSON(http.StatusInternalServerError, "admin grant service unavailable")
		return
	}

	grant, err := svc.GetByID(r.Request.Context(), grantID)
	if err != nil {
		r.ErrorJSON(http.StatusNotFound, "admin grant not found")
		return
	}

	// Build response
	resp := map[string]interface{}{
		"object":        "admin_grant",
		"id":            api.FormatAdminGrantID(grant.ID),
		"user_id":       grant.UserID,
		"price_id":      api.FormatPriceID(grant.PriceID),
		"granted_by":    grant.GrantedBy,
		"reason":        grant.Reason,
		"duration_days": grant.DurationDays,
		"created_at":    grant.CreatedAt.Unix(),
	}
	if grant.PaymentID != nil {
		resp["payment_id"] = api.FormatPaymentID(*grant.PaymentID)
	}
	if grant.Price != nil {
		resp["price"] = map[string]interface{}{
			"id":           api.FormatPriceID(grant.Price.ID),
			"display_name": grant.Price.DisplayName,
			"product_id":   api.FormatProductID(grant.Price.ProductID),
		}
	}

	r.GinCtx.JSON(http.StatusOK, resp)
}

// ListAdminGrantsByUser retrieves all admin grants for a user (admin action)
// GET /v1/admin/users/:user_id/grants
func ListAdminGrantsByUser(r *Request) {
	var path AdminUserEntitlementsPath
	if err := r.Inner().ShouldBindUri(&path); err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	svc := r.State.AdminGrantService
	if svc == nil {
		r.ErrorJSON(http.StatusInternalServerError, "admin grant service unavailable")
		return
	}

	// Parse pagination
	queryOpts := query.QueryOptions[any]{
		Limit:  50,
		Offset: 0,
	}
	if err := r.Inner().ShouldBindQuery(&queryOpts); err != nil {
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	grants, total, err := svc.ListByUserID(r.Request.Context(), path.UserID, queryOpts.Limit, queryOpts.Offset)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, "failed to fetch admin grants")
		return
	}

	// Build response
	data := make([]map[string]interface{}, 0, len(grants))
	for _, g := range grants {
		item := map[string]interface{}{
			"object":        "admin_grant",
			"id":            api.FormatAdminGrantID(g.ID),
			"user_id":       g.UserID,
			"price_id":      api.FormatPriceID(g.PriceID),
			"granted_by":    g.GrantedBy,
			"reason":        g.Reason,
			"duration_days": g.DurationDays,
			"created_at":    g.CreatedAt.Unix(),
		}
		if g.PaymentID != nil {
			item["payment_id"] = api.FormatPaymentID(*g.PaymentID)
		}
		data = append(data, item)
	}

	r.SuccessJSONPaginated(data, int64(total), queryOpts.Limit, queryOpts.Offset)
}
