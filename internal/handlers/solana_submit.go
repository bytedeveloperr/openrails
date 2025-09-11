package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/internal/utils/solana"
)

// SubmitPayment accepts a signed transaction and processes payment with real on-chain verification
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

	// Validate signature format
	if err := solana.ValidateSignature(req.SignedTransaction); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid transaction signature format")
		return
	}

	// Process payment with real on-chain verification
	user := r.GetUser()
	ctx, cancel := context.WithTimeout(r.Request.Context(), 30*time.Second)
	defer cancel()

	// Initialize real Solana services with config
	var rpcEndpoint, network string
	if r.State.Config.Solana != nil {
		rpcEndpoint = r.State.Config.Solana.RPCEndpoint
		network = r.State.Config.Solana.Network
	} else {
		network = "devnet" // fallback
	}
	rpcService := services.NewSolanaRPCService(rpcEndpoint, network)
	txService := services.NewSolanaTransactionService(r.State.DB, rpcService, r.State.Config, r.State.PriceService, r.State.PaymentService)

	// Verify transaction on-chain
	txResult, err := txService.VerifyTransactionSignature(ctx, req.SignedTransaction)
	if err != nil {
		log.WithError(err).Error("Failed to verify transaction on-chain")
		r.ErrorJSON(http.StatusBadRequest, fmt.Sprintf("Transaction verification failed: %v", err))
		return
	}

	// Get price for payment creation
	price, err := r.State.PriceService.GetByID(ctx, priceID)
	if err != nil {
		log.WithError(err).Error("Failed to get price for payment")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to process payment")
		return
	}

	// Create canonical payment record
	pay := &models.Payment{
		ID:            uuid.New(),
		UserID:        user.ID,
		PriceID:       price.ID,
		Processor:     models.ProcessorSolana,
		TransactionID: req.SignedTransaction,
		Amount:        price.Amount,
		Currency:      price.Currency,
		PurchasedAt:   time.Now(),
	}

	if err := r.State.PaymentService.Create(ctx, pay); err != nil {
		log.WithError(err).Error("Failed to create payment record")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to record payment")
		return
	}

	// Update any pending SolanaTransaction records
	_, _ = r.State.DB.GetDB().NewUpdate().
		TableExpr("solana_transactions").
		Set("status = ?", "confirmed").
		Set("signature = ?", req.SignedTransaction).
		Set("slot = ?", txResult.Slot).
		Set("block_time = ?", time.Unix(int64(*txResult.BlockTime), 0)).
		Where("user_id = ? AND amount = ?", user.ID, price.Amount).
		Where("status = ?", "pending").
		Exec(ctx)

	resp := SubmitPaymentResponse{
		PurchaseID:    pay.ID.String(),
		TransactionID: pay.TransactionID,
		Status:        "confirmed",
		Amount:        pay.Amount,
		Currency:      pay.Currency,
		ProcessedAt:   time.Now(),
		Message:       "Payment verified on-chain and processed successfully.",
	}

	log.WithFields(log.Fields{
		"payment_id":     pay.ID,
		"user_id":        user.ID,
		"transaction_id": req.SignedTransaction,
		"amount":         price.Amount,
		"slot":           txResult.Slot,
	}).Info("Solana payment successfully processed")

	r.SuccessJSON(resp)
}
