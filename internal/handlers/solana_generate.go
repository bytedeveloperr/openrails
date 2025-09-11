package handlers

import (
	"context"
	"net/http"
	"time"

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

	// Build via service (creates pending solana_transactions row and real transaction)
	user := r.GetUser()
	ctx, cancel := context.WithTimeout(r.Request.Context(), 15*time.Second)
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

	// Build real transaction
	txResp, err := txService.BuildPaymentTransaction(ctx, user.ID, priceID, req.Token, req.UserWallet)
	if err != nil {
		log.WithError(err).Error("Failed to build solana payment transaction")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to build payment transaction")
		return
	}

	// Simulate transaction to ensure it would succeed
	if err := txService.SimulateTransaction(ctx, txResp.Transaction); err != nil {
		log.WithError(err).Error("Transaction simulation failed")
		r.ErrorJSON(http.StatusBadRequest, "Transaction simulation failed - check wallet balance and token accounts")
		return
	}

	response := GeneratePaymentResponse{
		Transaction:  txResp.TransactionBase64,
		Amount:       txResp.Amount,
		Currency:     "USD", // TODO: Get from price
		TokenAmount:  txResp.TokenAmount,
		TokenSymbol:  txResp.TokenSymbol,
		ExpiresAt:    txResp.ExpiresAt.Unix(),
		Instructions: txResp.Instructions,
	}

	r.SuccessJSON(response)
}
