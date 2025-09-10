package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/services"
)

// SubmitPayment accepts a signed transaction and records a pending payment
// Note: This is a scaffold; real on-chain verification is not performed.
func SubmitPayment(r *Request) {
	req := new(SubmitPaymentRequest)
	if err := r.Bind(req); err != nil {
		log.WithError(err).Error("Failed to bind submit payment request")
		r.ErrorJSON(http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate price ID
	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID format")
		return
	}

	// Record payment via service
	user := r.GetUser()
	svc := services.NewSolanaPaymentService(r.State.DB, r.State.Config, r.State.PriceService, r.State.PaymentService)
	pay, err := svc.Submit(r.Request.Context(), user.ID, priceID, req.SignedTransaction)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}

	resp := SubmitPaymentResponse{
		PurchaseID:    pay.ID.String(),
		TransactionID: pay.TransactionID,
		Status:        "confirmed",
		Amount:        pay.Amount,
		Currency:      pay.Currency,
		ProcessedAt:   time.Now(),
		Message:       "Payment processed successfully.",
	}

	r.SuccessJSON(resp)
}
