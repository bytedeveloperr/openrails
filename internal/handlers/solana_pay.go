package handlers

import (
	"net/http"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/doujins-org/doujins-billing/pkg/api"
	log "github.com/sirupsen/logrus"
)

// SolanaPayRequest is the request body for POST /v1/solana/pay
type SolanaPayRequest struct {
	PriceID string `json:"price_id" binding:"required"`
	Token   string `json:"token" binding:"required"`
}

// SolanaPayResponse is the response for POST /v1/solana/pay
type SolanaPayResponse struct {
	URL         string `json:"url"`
	Reference   string `json:"reference"`
	Amount      int64  `json:"amount"` // cents
	Currency    string `json:"currency"`
	TokenAmount string `json:"token_amount"` // formatted (e.g., "9.99")
	Token       string `json:"token"`
	ExpiresAt   int64  `json:"expires_at"` // unix timestamp
}

// SolanaPayStatusResponse is the response for GET /v1/solana/pay/:reference
type SolanaPayStatusResponse struct {
	Status    string  `json:"status"` // "pending", "confirmed", "expired"
	PaymentID *string `json:"payment_id,omitempty"`
	Signature *string `json:"signature,omitempty"`
}

// CreateSolanaPay handles POST /v1/solana/pay
// Creates a new Solana Pay Transfer Request URL
func CreateSolanaPay(r *Request) {
	req := new(SolanaPayRequest)
	if !r.BindJSON(req) {
		return
	}

	// Parse price ID (strip prefix if present)
	priceID, err := api.ParsePriceID(req.PriceID)
	if err != nil {
		log.WithFields(log.Fields{"price_id": req.PriceID, "error": err.Error()}).Error("Invalid price ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID format")
		return
	}

	cl, ok := authgin.ClaimsFromGin(r.GinCtx)
	if !ok || cl.UserID == "" {
		r.ErrorJSON(http.StatusUnauthorized, "Authentication required")
		return
	}

	// Generate Solana Pay URL
	result, err := r.State.SolanaPayService.GeneratePayment(r.Request.Context(), cl.UserID, priceID, req.Token)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":  cl.UserID,
			"price_id": priceID,
			"token":    req.Token,
		}).Error("Failed to generate Solana Pay URL")
		r.ErrorJSON(http.StatusBadRequest, err.Error())
		return
	}

	log.WithFields(log.Fields{
		"user_id":   cl.UserID,
		"price_id":  priceID,
		"token":     req.Token,
		"reference": result.Reference,
	}).Info("Generated Solana Pay URL")

	r.SuccessJSON(SolanaPayResponse{
		URL:         result.URL,
		Reference:   result.Reference,
		Amount:      result.Amount,
		Currency:    result.Currency,
		TokenAmount: result.TokenAmount,
		Token:       result.Token,
		ExpiresAt:   result.ExpiresAt.Unix(),
	})
}

// GetSolanaPayByReference handles GET /v1/solana/pay/:reference
// Returns the status of a pending Solana payment by its reference key
func GetSolanaPayByReference(r *Request) {
	reference := r.GinCtx.Param("reference")
	if reference == "" {
		r.ErrorJSON(http.StatusBadRequest, "reference is required")
		return
	}

	status, payment, err := r.State.SolanaPayService.GetPaymentStatus(r.Request.Context(), reference)
	if err != nil {
		log.WithError(err).WithField("reference", reference).Error("Failed to get payment status")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to check payment status")
		return
	}

	resp := SolanaPayStatusResponse{
		Status: status,
	}

	if payment != nil {
		paymentID := api.FormatPaymentID(payment.ID)
		resp.PaymentID = &paymentID
		resp.Signature = &payment.TransactionID
	}

	r.SuccessJSON(resp)
}
