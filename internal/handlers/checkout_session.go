package handlers

import (
	"net/http"
	"strings"
)

// CreateCheckoutSession handles POST /v1/checkout
func CreateCheckoutSession(r *Request) {
	var req CheckoutSessionCreateRequest
	if !r.BindJSON(&req) {
		return
	}

	r.ErrorJSON(http.StatusNotImplemented, "checkout sessions are not implemented yet")
}

// GetCheckoutSession handles GET /v1/checkout/:id
func GetCheckoutSession(r *Request) {
	sessionID := strings.TrimSpace(r.GinCtx.Param("id"))
	if sessionID == "" {
		r.ErrorJSON(http.StatusBadRequest, "id is required")
		return
	}

	r.ErrorJSON(http.StatusNotImplemented, "checkout sessions are not implemented yet")
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

	r.ErrorJSON(http.StatusNotImplemented, "checkout session confirmation is not implemented yet")
}
