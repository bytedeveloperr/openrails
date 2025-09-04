package handlers

import (
    "fmt"
    "math"
    "net/http"
    "time"

    "github.com/google/uuid"
    log "github.com/sirupsen/logrus"
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

    // Load price
    price, err := r.State.PriceService.GetByID(r.Request.Context(), priceID)
    if err != nil {
        log.WithFields(log.Fields{"price_id": priceID, "error": err.Error()}).Error("Failed to get price information")
        r.ErrorJSON(http.StatusNotFound, "Price not found")
        return
    }

    // Validate token via config
    solCfg := r.State.Config.Solana
    if solCfg == nil {
        log.Error("Solana configuration not found")
        r.ErrorJSON(http.StatusInternalServerError, "Payment system configuration error")
        return
    }

    tokenSymbol := req.Token
    tokenCfg, ok := solCfg.SupportedTokens[tokenSymbol]
    if !ok || !tokenCfg.Enabled {
        r.ErrorJSON(http.StatusBadRequest, "Invalid or unsupported token")
        return
    }

    // Calculate token amount in smallest units
    pow := math.Pow10(tokenCfg.Decimals)
    tokenAmount := uint64(math.Round(price.Amount * pow))

    response := GeneratePaymentResponse{
        Transaction:  "", // Real transaction generation requires on-chain libs; intentionally empty
        Amount:       price.Amount,
        Currency:     price.Currency,
        TokenAmount:  tokenAmount,
        TokenSymbol:  tokenSymbol,
        ExpiresAt:    time.Now().Add(10 * time.Minute).Unix(),
        Instructions: fmt.Sprintf("Please sign this transaction to pay %.2f %s using %s.", price.Amount, price.Currency, tokenSymbol),
    }

    log.WithFields(log.Fields{
        "price_id":     priceID,
        "amount":       price.Amount,
        "token_symbol": tokenSymbol,
        "token_amount": tokenAmount,
    }).Info("Prepared Solana payment transaction response")

    r.SuccessJSON(response)
}
