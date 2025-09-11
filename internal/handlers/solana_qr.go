package handlers

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/services"
)

// GenerateSolanaPayQR generates a Solana Pay URL for wallet apps to scan
func GenerateSolanaPayQR(r *Request) {
	req := new(GenerateSolanaPayQRRequest)
	if err := r.Bind(req); err != nil {
		log.WithError(err).Error("Failed to bind generate QR request")
		r.ErrorJSON(http.StatusBadRequest, "Invalid request parameters")
		return
	}

	// Parse price ID
	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithFields(log.Fields{"price_id": req.PriceID, "error": err.Error()}).Error("Invalid price ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID format")
		return
	}

	// Get price
	ctx, cancel := context.WithTimeout(r.Request.Context(), 10*time.Second)
	defer cancel()
	
	price, err := r.State.PriceService.GetByID(ctx, priceID)
	if err != nil {
		log.WithFields(log.Fields{"price_id": priceID, "error": err.Error()}).Error("Failed to get price information")
		r.ErrorJSON(http.StatusNotFound, "Price not found")
		return
	}

	// Config
	solCfg := r.State.Config.Solana
	if solCfg == nil {
		log.Error("Solana configuration not found")
		r.ErrorJSON(http.StatusInternalServerError, "Payment system configuration error")
		return
	}

	// Token
	tokenSymbol := req.Token
	tokenCfg, ok := solCfg.SupportedTokens[tokenSymbol]
	if !ok || !tokenCfg.Enabled {
		r.ErrorJSON(http.StatusBadRequest, "Invalid or unsupported token")
		return
	}

	// Merchant wallet
	merchantWallet := solCfg.RecipientWallet
	if merchantWallet == "" {
		merchantWallet = solCfg.DestinationWallet
	}
	if merchantWallet == "" {
		log.Error("Solana recipient wallet not configured")
		r.ErrorJSON(http.StatusInternalServerError, "Payment system configuration error")
		return
	}

	// Create pending solana transaction (captures reference via pending ID)
	user := r.GetUser()
	svc := services.NewSolanaPaymentService(r.State.DB, r.State.Config, r.State.PriceService, r.State.PaymentService)
	_, _, _, exp, pendingID, err := svc.Generate(ctx, user.ID, price.ID, tokenSymbol, req.UserWallet)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare payment")
		return
	}

	// Calculate smallest unit amount
	tokenAmount := price.Amount * math.Pow10(tokenCfg.Decimals)

	// Build Solana Pay URL
	solanaPayURL := url.URL{Scheme: "solana", Host: merchantWallet}
	params := url.Values{}
	params.Set("amount", fmt.Sprintf("%.0f", tokenAmount))
	params.Set("spl-token", tokenCfg.Mint)
	// Include a reference (pending ID) to correlate chain tx to server record
	params.Add("reference", pendingID.String())
	params.Set("label", "Doujins Purchase")
	params.Set("message", fmt.Sprintf("Purchase for %.2f %s using %s", price.Amount, price.Currency, tokenSymbol))
	solanaPayURL.RawQuery = params.Encode()

	resp := SolanaPayQRResponse{
		URL:         solanaPayURL.String(),
		Amount:      price.Amount,
		TokenAmount: fmt.Sprintf("%.2f", price.Amount),
		TokenSymbol: tokenSymbol,
		Label:       "Doujins Purchase",
		Message:     fmt.Sprintf("Purchase for %.2f %s using %s", price.Amount, price.Currency, tokenSymbol),
		ExpiresAt:   exp.Unix(),
	}

	log.WithFields(log.Fields{
		"price_id":        priceID,
		"amount":          price.Amount,
		"token_symbol":    tokenSymbol,
		"merchant_wallet": merchantWallet,
		"user_wallet":     req.UserWallet,
	}).Info("Generated Solana Pay QR code URL")

	r.SuccessJSON(resp)
}
