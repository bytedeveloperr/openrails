package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/api"
)

// CreateCheckoutSession handles POST /v1/checkout
func CreateCheckoutSession(r *Request) {
	var req CheckoutSessionCreateRequest
	if !r.BindJSON(&req) {
		return
	}

	user := r.GetUser()
	if user == nil || strings.TrimSpace(user.ID) == "" {
		r.ErrorJSON(http.StatusUnauthorized, "authentication required")
		return
	}
	if r.State.CheckoutSessionService == nil {
		r.ErrorJSON(http.StatusInternalServerError, "checkout session service unavailable")
		return
	}

	req.IdempotencyKey = r.GinCtx.GetHeader("X-Idempotency-Key")
	svcReq := &services.CheckoutSessionCreateRequest{
		PriceID:        req.PriceID,
		Mode:           req.Mode,
		Metadata:       req.Metadata,
		IdempotencyKey: req.IdempotencyKey,
		Payment: services.CheckoutSessionPaymentRequest{
			Processor:       req.Payment.Processor,
			PaymentMethodID: req.Payment.PaymentMethodID,
			PaymentToken:    req.Payment.PaymentToken,
			TokenSymbol:     req.Payment.TokenSymbol,
			Flow:            req.Payment.Flow,
			Wallet:          req.Payment.Wallet,
			Email:           req.Payment.Email,
			FirstName:       req.Payment.FirstName,
			LastName:        req.Payment.LastName,
			Address1:        req.Payment.Address1,
			City:            req.Payment.City,
			State:           req.Payment.State,
			Zip:             req.Payment.Zip,
			Country:         req.Payment.Country,
		},
	}

	resp, err := r.State.CheckoutSessionService.CreateSession(r.Request.Context(), svcReq, user)
	if err != nil {
		writeCheckoutSessionError(r, err)
		return
	}

	r.SuccessJSON(resp)
}

// GetCheckoutSession handles GET /v1/checkout/:id
func GetCheckoutSession(r *Request) {
	sessionID := strings.TrimSpace(r.GinCtx.Param("id"))
	if sessionID == "" {
		r.ErrorJSON(http.StatusBadRequest, "id is required")
		return
	}

	user := r.GetUser()
	if user == nil || strings.TrimSpace(user.ID) == "" {
		r.ErrorJSON(http.StatusUnauthorized, "authentication required")
		return
	}
	if r.State.CheckoutSessionService == nil {
		r.ErrorJSON(http.StatusInternalServerError, "checkout session service unavailable")
		return
	}

	parsedID, err := api.ParseCheckoutSessionID(sessionID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid checkout session id")
		return
	}

	resp, err := r.State.CheckoutSessionService.GetSession(r.Request.Context(), parsedID, user)
	if err != nil {
		writeCheckoutSessionError(r, err)
		return
	}

	r.SuccessJSON(resp)
}

// ConfirmCheckoutSession handles POST /v1/checkout/:id/confirm
func ConfirmCheckoutSession(r *Request) {
	sessionID := strings.TrimSpace(r.GinCtx.Param("id"))
	if sessionID == "" {
		r.ErrorJSON(http.StatusBadRequest, "id is required")
		return
	}

	var req CheckoutSessionConfirmRequest
	if !r.BindJSON(&req) {
		return
	}

	user := r.GetUser()
	if user == nil || strings.TrimSpace(user.ID) == "" {
		r.ErrorJSON(http.StatusUnauthorized, "authentication required")
		return
	}
	if r.State.CheckoutSessionService == nil {
		r.ErrorJSON(http.StatusInternalServerError, "checkout session service unavailable")
		return
	}

	parsedID, err := api.ParseCheckoutSessionID(sessionID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid checkout session id")
		return
	}

	svcReq := &services.CheckoutSessionConfirmRequest{
		Payment: services.CheckoutSessionConfirmPayment{
			Processor: req.Payment.Processor,
			Signature: req.Payment.Signature,
			Wallet:    req.Payment.Wallet,
		},
	}

	resp, err := r.State.CheckoutSessionService.ConfirmSession(r.Request.Context(), parsedID, svcReq, user)
	if err != nil {
		writeCheckoutSessionError(r, err)
		return
	}

	r.SuccessJSON(resp)
}

func writeCheckoutSessionError(r *Request, err error) {
	switch {
	case errors.Is(err, services.ErrCheckoutSessionNotFound):
		r.ErrorJSON(http.StatusNotFound, err.Error())
	case errors.Is(err, services.ErrCheckoutSessionForbidden):
		r.ErrorJSON(http.StatusForbidden, err.Error())
	case errors.Is(err, services.ErrCheckoutSessionExpired):
		r.ErrorJSON(http.StatusGone, err.Error())
	case errors.Is(err, services.ErrCheckoutSessionPending):
		r.ErrorJSON(http.StatusConflict, err.Error())
	case errors.Is(err, services.ErrCheckoutSessionConflict):
		r.ErrorJSON(http.StatusConflict, err.Error())
	case errors.Is(err, services.ErrCheckoutSessionValidation):
		r.ErrorJSON(http.StatusBadRequest, err.Error())
	default:
		r.ErrorJSON(http.StatusInternalServerError, "checkout session request failed")
	}
}
