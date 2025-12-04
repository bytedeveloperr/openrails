package handlers

import (
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/services"
)

// Checkout handles unified checkout for both subscriptions and one-time purchases
// POST /v1/me/checkout
//
// Request body:
//
//	{
//	  "price_id": "uuid",            // Required: Price to purchase
//	  "processor": "mobius|ccbill|solana", // Required: Payment processor
//	  "payment_method_id": "uuid",   // Optional: Stored payment method (for mobius)
//	  "payment_token": "string",     // Optional: Collect.js token (for mobius)
//	  "email": "string",             // Optional: Email for billing
//	  "first_name": "string",        // Optional: First name for billing
//	  "last_name": "string",         // Optional: Last name for billing
//	  "address1": "string",          // Optional: Address for billing
//	  "city": "string",              // Optional: City for billing
//	  "state": "string",             // Optional: State for billing
//	  "zip": "string",               // Optional: Zip for billing
//	  "country": "string"            // Optional: Country code (2 letters)
//	}
//
// Response:
//
//	{
//	  "status": "success|pending|redirect_required|blocked",
//	  "message": "string",
//	  "subscription_id": "uuid",     // For subscription purchases
//	  "payment_id": "uuid",          // For one-time purchases
//	  "transaction_id": "string",    // Processor transaction ID
//	  "redirect_url": "string",      // For CCBill FlexForm
//	  "delayed_start": "datetime"    // When purchase takes effect (if delayed)
//	}
//
// Behavior:
//   - If price.billing_cycle_days != nil → subscription flow
//   - If price.billing_cycle_days == nil → one-time purchase flow
//   - Checks for existing coverage (subscriptions + entitlements)
//   - If user has indefinite coverage → blocks purchase
//   - If user has coverage with end date → allows with delayed start
//   - CCBill: cannot delay start, blocks if user has existing coverage
//   - NMI/mobius: creates subscription with start_date = coverage_end_date
//   - Solana: charges immediately, delays entitlement start to coverage_end_date
func Checkout(r *Request) {
	var req CheckoutRequest
	if !r.BindJSON(&req) {
		return
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}

	if r.State.CheckoutService == nil {
		log.Error("Checkout service is not configured")
		r.ErrorJSON(http.StatusInternalServerError, "Checkout service unavailable")
		return
	}

	// Convert handler request to service request
	checkoutReq := &services.CheckoutRequest{
		PriceID:         req.PriceID,
		PaymentMethodID: req.PaymentMethodID,
		PaymentToken:    req.PaymentToken,
		Processor:       req.Processor,
		Email:           req.Email,
		FirstName:       req.FirstName,
		LastName:        req.LastName,
		Address1:        req.Address1,
		City:            req.City,
		State:           req.State,
		Zip:             req.Zip,
		Country:         req.Country,
	}

	resp, err := r.State.CheckoutService.Checkout(r.Request.Context(), checkoutReq, userCtx.User)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":   userCtx.User.ID,
			"price_id":  req.PriceID,
			"processor": req.Processor,
		}).Error("Checkout failed")
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	// Determine HTTP status based on response status
	httpStatus := http.StatusOK
	if resp.Status == "blocked" {
		httpStatus = http.StatusConflict
	}

	r.GinCtx.JSON(httpStatus, resp)
}
