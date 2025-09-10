package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/services"
)

// GeneratePayment handles generating Solana payment transactions (scaffold)
// Note: This implementation validates inputs and returns a structured response,
// but does not create a real Solana transaction to avoid external deps.
func GeneratePayment(r *Request) {
	req := new(GeneratePaymentRequest)
	if err := r.Bind(req); err != nil {
		log.WithError(err).Error("Failed to bind generate payment request")
		r.ErrorJSON(http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate price ID
	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithFields(log.Fields{"price_id": req.PriceID, "error": err.Error()}).Error("Invalid price ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID format")
		return
	}

	// Build via service (creates pending solana_transactions row)
	user := r.GetUser()
	svc := services.NewSolanaPaymentService(r.State.DB, r.State.Config, r.State.PriceService, r.State.PaymentService)
	amount, currency, tokenAmount, expiresAt, _, err := svc.Generate(r.Request.Context(), user.ID, priceID, req.Token, req.UserWallet)
	if err != nil {
		log.WithError(err).Error("Failed to prepare solana payment")
		if errors.Is(err, services.ErrPriceNotFound) {
			r.ErrorJSON(http.StatusNotFound, "Price not found")
			return
		}
		if errors.Is(err, services.ErrInvalidToken) {
			r.ErrorJSON(http.StatusBadRequest, "Invalid or unsupported token")
			return
		}
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare payment")
		return
	}

	response := GeneratePaymentResponse{
		Transaction:  "", // intentionally empty until on-chain builder is wired
		Amount:       amount,
		Currency:     currency,
		TokenAmount:  tokenAmount,
		TokenSymbol:  req.Token,
		ExpiresAt:    expiresAt.Unix(),
		Instructions: fmt.Sprintf("Please sign this transaction to pay %.2f %s using %s.", amount, currency, req.Token),
	}

	r.SuccessJSON(response)
}
