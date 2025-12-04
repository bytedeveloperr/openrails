package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	solanago "github.com/doujins-org/solana-go"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/services"
	solanaUtils "github.com/doujins-org/doujins-billing/internal/utils/solana"
	"github.com/doujins-org/doujins-billing/pkg/api"
)

// CreatePaymentIntent creates a new payment intent for direct wallet flow
// POST /v1/payment-intents
func CreatePaymentIntent(r *Request) {
	req := new(CreatePaymentIntentRequest)
	if !r.BindJSON(req) {
		return
	}

	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithFields(log.Fields{"price_id": req.PriceID, "error": err.Error()}).Error("Invalid price ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Request.Context(), 15*time.Second)
	defer cancel()

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}
	user := userCtx.User

	wallet, err := r.State.SolanaWalletService.Get(ctx, user.ID, req.Wallet)
	if err != nil {
		if errors.Is(err, services.ErrWalletNotFound) {
			log.WithFields(log.Fields{"user_id": user.ID, "wallet": req.Wallet}).Warn("Wallet not linked to user")
			r.ErrorJSON(http.StatusBadRequest, "Wallet is not linked to this account")
			return
		}
		log.WithError(err).Error("Failed to load Solana wallet for transaction generation")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to load wallet")
		return
	}
	if !wallet.IsVerified {
		log.WithFields(log.Fields{"user_id": user.ID, "wallet": wallet.Address}).Warn("Attempted to generate transaction with unverified wallet")
		r.ErrorJSON(http.StatusBadRequest, "Wallet must be verified before generating a transaction")
		return
	}

	amount, currency, tokenAmount, expiresAt, err := r.State.SolanaPaymentService.Generate(ctx, user.ID, priceID, req.Token, wallet.Address)
	if err != nil {
		log.WithError(err).Error("Failed to prepare Solana payment quote")
		r.ErrorJSON(http.StatusBadRequest, "Unable to generate payment for this price")
		return
	}

	intent, err := r.State.SolanaPaymentIntentService.CreateDirectIntent(ctx, user.ID, priceID, req.Token, wallet.Address)
	if err != nil {
		log.WithError(err).Error("Failed to create Solana payment intent")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare payment")
		return
	}

	var rpcEndpoint, network string
	if r.State.Config.Solana != nil {
		rpcEndpoint = r.State.Config.Solana.RPCEndpoint
		network = r.State.Config.Solana.Network
	} else {
		network = "devnet"
	}

	rpcService := services.NewSolanaRPCService(rpcEndpoint, network)
	txService := services.NewSolanaTransactionService(r.State.DB, rpcService, r.State.Config, r.State.PriceService, r.State.PaymentService)

	txResp, err := txService.BuildPaymentTransaction(ctx, user.ID, priceID, req.Token, wallet.Address)
	if err != nil {
		log.WithError(err).Error("Failed to build solana payment transaction")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to build payment transaction")
		return
	}

	// Get token mint from config
	var tokenMint string
	if r.State.Config.Solana != nil {
		if tokenCfg, ok := r.State.Config.Solana.SupportedTokens[req.Token]; ok {
			tokenMint = tokenCfg.Mint
		}
	}

	response := api.PaymentIntentObject{
		ID:       api.FormatPaymentIntentID(intent.ID),
		Object:   "payment_intent",
		Status:   string(intent.Status),
		Amount:   amount,
		Currency: currency,
		PaymentMethod: &api.PaymentIntentPaymentMethod{
			Type:        "solana",
			Token:       req.Token,
			TokenMint:   tokenMint,
			TokenAmount: formatTokenAmountFromUnits(tokenAmount, getTokenDecimals(r, req.Token)),
			Wallet:      wallet.Address,
		},
		Transaction: &api.PaymentIntentTransaction{
			Data: txResp.TransactionBase64,
		},
		ExpiresAt: expiresAt.Unix(),
		Created:   intent.CreatedAt.Unix(),
	}

	r.SuccessJSON(response)
}

// CreatePaymentIntentQR creates a new payment intent for Solana Pay QR flow
// POST /v1/payment-intents/qr
func CreatePaymentIntentQR(r *Request) {
	req := new(CreatePaymentIntentQRRequest)
	if !r.BindJSON(req) {
		return
	}

	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithFields(log.Fields{"price_id": req.PriceID, "error": err.Error()}).Error("Invalid price ID format")
		r.ErrorJSON(http.StatusBadRequest, "Invalid price ID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Request.Context(), 10*time.Second)
	defer cancel()

	price, err := r.State.PriceService.GetByID(ctx, priceID)
	if err != nil {
		log.WithFields(log.Fields{"price_id": priceID, "error": err.Error()}).Error("Failed to get price information")
		r.ErrorJSON(http.StatusNotFound, "Price not found")
		return
	}

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

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}
	user := userCtx.User

	amount, _, tokenAmountUnits, exp, err := r.State.SolanaPaymentService.Generate(ctx, user.ID, price.ID, tokenSymbol, req.Wallet)
	if err != nil {
		log.WithError(err).Error("Failed to create Solana Pay intent")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare payment")
		return
	}

	intent, err := r.State.SolanaPaymentIntentService.CreateSolanaPayIntent(ctx, user.ID, price.ID, tokenSymbol)
	if err != nil {
		log.WithError(err).Error("Failed to create Solana Pay intent")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare payment")
		return
	}

	if intent.Reference == nil {
		log.WithField("intent_id", intent.ID).Error("Solana Pay intent missing reference")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to prepare payment")
		return
	}

	merchantWallet := intent.RecipientWallet
	params := url.Values{}
	params.Set("amount", formatTokenAmountPI(tokenAmountUnits, tokenCfg.Decimals))
	if tokenCfg.Mint != "" && strings.ToUpper(tokenSymbol) != "SOL" {
		params.Set("spl-token", tokenCfg.Mint)
	}
	params.Add("reference", *intent.Reference)
	params.Set("label", "Doujins Purchase")
	if intent.Memo != nil {
		params.Set("memo", *intent.Memo)
	}
	solanaURL := fmt.Sprintf("solana:%s", merchantWallet)
	if encoded := params.Encode(); encoded != "" {
		solanaURL = fmt.Sprintf("%s?%s", solanaURL, encoded)
	}

	response := api.PaymentIntentObject{
		ID:       api.FormatPaymentIntentID(intent.ID),
		Object:   "payment_intent",
		Status:   string(intent.Status),
		Amount:   amount,
		Currency: "usd",
		PaymentMethod: &api.PaymentIntentPaymentMethod{
			Type:        "solana",
			Token:       tokenSymbol,
			TokenMint:   tokenCfg.Mint,
			TokenAmount: formatTokenAmountPI(tokenAmountUnits, tokenCfg.Decimals),
		},
		Transaction: &api.PaymentIntentTransaction{
			URL:       solanaURL,
			Reference: *intent.Reference,
		},
		ExpiresAt: exp.Unix(),
		Created:   intent.CreatedAt.Unix(),
	}

	log.WithFields(log.Fields{
		"price_id":        priceID,
		"amount":          price.Amount,
		"token_symbol":    tokenSymbol,
		"merchant_wallet": merchantWallet,
		"reference":       *intent.Reference,
	}).Info("Generated Solana Pay payment intent")

	r.SuccessJSON(response)
}

// GetPaymentIntent retrieves the status of a payment intent
// GET /v1/payment-intents/:id
func GetPaymentIntent(r *Request) {
	idParam := r.Param("id")

	intentID, err := api.ParsePaymentIntentID(idParam)
	if err != nil {
		// Try parsing as raw UUID for backwards compatibility
		intentID, err = uuid.Parse(idParam)
		if err != nil {
			r.ErrorJSON(http.StatusBadRequest, "Invalid payment intent ID format")
			return
		}
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}
	user := userCtx.User

	ctx, cancel := context.WithTimeout(r.Request.Context(), 30*time.Second)
	defer cancel()

	intent, err := r.State.SolanaPaymentIntentService.GetByID(ctx, intentID)
	if err != nil {
		log.WithError(err).Warn("Payment intent not found")
		r.ErrorJSON(http.StatusNotFound, "Payment intent not found")
		return
	}

	if intent.UserID != user.ID {
		log.WithFields(log.Fields{"intent_id": intent.ID, "user_id": user.ID}).Warn("Attempt to access intent belonging to another user")
		r.ErrorJSON(http.StatusForbidden, "Payment intent does not belong to you")
		return
	}

	// If this is a QR flow intent that's still pending, check for on-chain confirmation
	if intent.FlowType == services.FlowTypeSolanaPay && intent.Status == services.IntentStatusPending && intent.Reference != nil {
		if err := r.checkAndConfirmQRPayment(ctx, intent, user); err != nil {
			log.WithError(err).Debug("QR payment check did not find confirmed transaction")
		} else {
			// Re-fetch the intent after potential update
			intent, _ = r.State.SolanaPaymentIntentService.GetByID(ctx, intentID)
		}
	}

	response := buildPaymentIntentResponse(intent, r)
	r.SuccessJSON(response)
}

// ConfirmPaymentIntent confirms a payment intent by submitting a signed transaction
// POST /v1/payment-intents/:id/confirm
func ConfirmPaymentIntent(r *Request) {
	idParam := r.Param("id")

	intentID, err := api.ParsePaymentIntentID(idParam)
	if err != nil {
		// Try parsing as raw UUID for backwards compatibility
		intentID, err = uuid.Parse(idParam)
		if err != nil {
			r.ErrorJSON(http.StatusBadRequest, "Invalid payment intent ID format")
			return
		}
	}

	req := new(ConfirmPaymentIntentRequest)
	if !r.BindJSON(req) {
		return
	}

	userCtx := middleware.GetUserContext(r.GinCtx)
	if userCtx.User == nil {
		r.ErrorJSON(http.StatusUnauthorized, "User authentication required")
		return
	}
	user := userCtx.User

	ctx, cancel := context.WithTimeout(r.Request.Context(), 30*time.Second)
	defer cancel()

	intent, err := r.State.SolanaPaymentIntentService.GetByID(ctx, intentID)
	if err != nil {
		log.WithError(err).Error("Failed to load payment intent")
		r.ErrorJSON(http.StatusBadRequest, "Invalid payment intent")
		return
	}
	if intent.UserID != user.ID {
		log.WithFields(log.Fields{"intent_id": intent.ID, "user_id": user.ID}).Warn("Intent does not belong to user")
		r.ErrorJSON(http.StatusForbidden, "Payment intent does not belong to you")
		return
	}
	if intent.FlowType != services.FlowTypeDirect {
		log.WithFields(log.Fields{"intent_id": intent.ID, "flow": intent.FlowType}).Warn("Confirm called for non-direct intent")
		r.ErrorJSON(http.StatusBadRequest, "Payment intent is not for direct wallet flow")
		return
	}
	if intent.Status != services.IntentStatusPending && intent.Status != services.IntentStatusProcessing {
		log.WithFields(log.Fields{"intent_id": intent.ID, "status": intent.Status}).Warn("Intent already processed")
		r.ErrorJSON(http.StatusBadRequest, "Payment intent already processed")
		return
	}
	if r.State.SolanaPaymentIntentService.IsExpired(intent) {
		log.WithFields(log.Fields{"intent_id": intent.ID}).Warn("Intent expired")
		r.ErrorJSON(http.StatusBadRequest, "Payment intent expired")
		return
	}

	if err = r.State.SolanaPaymentIntentService.MarkProcessing(ctx, intent.ID); err != nil && !errors.Is(err, services.ErrIntentInvalidState) {
		log.WithError(err).Warn("Failed to transition intent to processing state")
	}

	if intent.PayerWallet == nil || *intent.PayerWallet == "" {
		log.WithFields(log.Fields{"intent_id": intent.ID}).Error("Direct intent missing payer wallet")
		r.ErrorJSON(http.StatusInternalServerError, "Payment intent misconfigured")
		return
	}

	userWallet, err := r.State.SolanaWalletService.Get(ctx, user.ID, *intent.PayerWallet)
	if err != nil {
		log.WithError(err).Error("Failed to load payer wallet for intent")
		r.ErrorJSON(http.StatusBadRequest, "Linked wallet not available")
		return
	}
	if !userWallet.IsVerified {
		log.WithFields(log.Fields{"user_id": user.ID, "wallet": userWallet.Address}).Warn("Confirm payment attempted with unverified wallet")
		r.ErrorJSON(http.StatusBadRequest, "Wallet must be verified before confirming a payment")
		return
	}

	var rpcEndpoint, network string
	if r.State.Config.Solana != nil {
		rpcEndpoint = r.State.Config.Solana.RPCEndpoint
		network = r.State.Config.Solana.Network
	} else {
		network = "devnet"
	}
	rpcService := services.NewSolanaRPCService(rpcEndpoint, network)
	txService := services.NewSolanaTransactionService(r.State.DB, rpcService, r.State.Config, r.State.PriceService, r.State.PaymentService)

	signature := req.SignedTransaction
	var decodedTx *solanago.Transaction
	if txBytes, decodeErr := base64.StdEncoding.DecodeString(req.SignedTransaction); decodeErr == nil {
		if parsedTx, parseErr := solanago.TransactionFromBytes(txBytes); parseErr == nil {
			decodedTx = parsedTx
		} else {
			log.WithError(parseErr).Warn("Provided transaction payload could not be parsed; falling back to signature verification")
		}
	}

	if decodedTx != nil {
		if len(decodedTx.Message.AccountKeys) == 0 {
			r.ErrorJSON(http.StatusBadRequest, "Signed transaction missing fee payer")
			return
		}
		feePayer := decodedTx.Message.AccountKeys[0].String()
		if feePayer != userWallet.Address {
			log.WithFields(log.Fields{
				"user_id":           user.ID,
				"linked_wallet":     userWallet.Address,
				"transaction_payer": feePayer,
			}).Warn("Signed transaction does not originate from linked wallet")
			r.ErrorJSON(http.StatusBadRequest, "Signed transaction must use your linked wallet as fee payer")
			return
		}

		if len(decodedTx.Signatures) == 0 {
			r.ErrorJSON(http.StatusBadRequest, "Signed transaction does not contain a signature")
			return
		}

		if sentSig, sendErr := rpcService.SendTransaction(ctx, decodedTx); sendErr != nil {
			log.WithError(sendErr).Warn("Failed to broadcast signed transaction; continuing with local signature")
			signature = decodedTx.Signatures[0].String()
		} else {
			signature = sentSig.String()
		}
	}

	if err = solanaUtils.ValidateSignature(signature); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid transaction signature format")
		return
	}

	txResult, err := txService.VerifyTransactionWithContent(ctx, signature, intent.ExpectedAmountLamports, intent.RecipientWallet, intent.TokenMint, userWallet.Address, intent.Reference)
	if err != nil {
		log.WithError(err).Error("Failed to verify transaction on-chain")
		_ = r.State.SolanaPaymentIntentService.MarkFailed(ctx, intent.ID, err.Error())
		r.ErrorJSON(http.StatusBadRequest, fmt.Sprintf("Transaction verification failed: %v", err))
		return
	}

	if err = r.State.SolanaPaymentIntentService.MarkConfirmed(ctx, intent.ID, signature); err != nil {
		log.WithError(err).Warn("Failed to mark payment intent confirmed")
	}

	pay, err := r.State.SolanaPaymentService.Submit(ctx, user.ID, intent.ID, intent.PriceID, signature, user.Email)
	if err != nil {
		log.WithError(err).Error("Failed to process payment and grant entitlements")
		_ = r.State.SolanaPaymentIntentService.MarkFailed(ctx, intent.ID, err.Error())
		r.ErrorJSON(http.StatusInternalServerError, "Failed to process payment")
		return
	}

	txDetails := extractTransactionDetails(txResult, userWallet.Address, intent.Token, intent.TokenMint)

	userIDCopy := user.ID
	solanaTransaction := &models.SolanaTransaction{
		ID:          uuid.New(),
		UserID:      &userIDCopy,
		Signature:   &signature,
		Status:      "confirmed",
		Amount:      intent.Amount,
		Token:       txDetails.Token,
		TokenMint:   txDetails.TokenMint,
		FromAddress: txDetails.FromAddress,
		ToAddress:   intent.RecipientWallet,
		IntentID:    &intent.ID,
		BlockTime: func() *time.Time {
			if txResult.BlockTime != nil {
				t := time.Unix(int64(*txResult.BlockTime), 0)
				return &t
			}
			return nil
		}(),
		Slot: func() *int64 {
			if txResult.Slot > 0 {
				s := int64(txResult.Slot)
				return &s
			}
			return nil
		}(),
	}

	if _, err := r.State.DB.GetDB().NewInsert().Model(solanaTransaction).Exec(ctx); err != nil {
		log.WithError(err).Error("Failed to create SolanaTransaction record")
	}

	log.WithFields(log.Fields{
		"payment_id":     pay.ID,
		"user_id":        user.ID,
		"transaction_id": signature,
		"amount":         pay.Amount,
		"slot":           txResult.Slot,
	}).Info("Solana payment successfully processed via confirm endpoint")

	// Re-fetch intent to get updated status
	intent, _ = r.State.SolanaPaymentIntentService.GetByID(ctx, intent.ID)

	response := buildPaymentIntentResponse(intent, r)
	response.Transaction.Signature = signature
	r.SuccessJSON(response)
}

// checkAndConfirmQRPayment checks for on-chain confirmation of a QR payment and processes it
func (r *Request) checkAndConfirmQRPayment(ctx context.Context, intent *models.SolanaPaymentIntent, user *services.UserIdentity) error {
	if intent.Reference == nil {
		return errors.New("no reference for QR intent")
	}

	var rpcEndpoint, network string
	if r.State.Config.Solana != nil {
		rpcEndpoint = r.State.Config.Solana.RPCEndpoint
		network = r.State.Config.Solana.Network
	} else {
		network = "devnet"
	}
	rpcService := services.NewSolanaRPCService(rpcEndpoint, network)
	txService := services.NewSolanaTransactionService(r.State.DB, rpcService, r.State.Config, r.State.PriceService, r.State.PaymentService)

	refPubkey, err := solanago.PublicKeyFromBase58(*intent.Reference)
	if err != nil {
		return fmt.Errorf("invalid reference: %w", err)
	}

	limit := 10
	signatures, err := rpcService.GetSignaturesForAddress(ctx, refPubkey, &limit)
	if err != nil {
		return fmt.Errorf("failed to check for transactions: %w", err)
	}

	if len(signatures) == 0 {
		return errors.New("no transactions found")
	}

	signature := signatures[0].Signature
	signatureStr := signature.String()

	txResult, err := txService.VerifyTransactionWithContent(ctx, signatureStr, intent.ExpectedAmountLamports, intent.RecipientWallet, intent.TokenMint, "", intent.Reference)
	if err != nil {
		_ = r.State.SolanaPaymentIntentService.MarkFailed(ctx, intent.ID, err.Error())
		return fmt.Errorf("transaction verification failed: %w", err)
	}

	if err = r.State.SolanaPaymentIntentService.MarkConfirmed(ctx, intent.ID, signatureStr); err != nil {
		log.WithError(err).Warn("Failed to mark Solana Pay intent confirmed")
	}

	_, err = r.State.SolanaPaymentService.Submit(ctx, user.ID, intent.ID, intent.PriceID, signatureStr, user.Email)
	if err != nil {
		_ = r.State.SolanaPaymentIntentService.MarkFailed(ctx, intent.ID, err.Error())
		return fmt.Errorf("failed to record payment: %w", err)
	}

	txDetails := extractTransactionDetails(txResult, "", intent.Token, intent.TokenMint)

	userIDCopy := user.ID
	solanaTransaction := &models.SolanaTransaction{
		ID:          uuid.New(),
		UserID:      &userIDCopy,
		Signature:   &signatureStr,
		Status:      "confirmed",
		Amount:      intent.Amount,
		Token:       txDetails.Token,
		TokenMint:   txDetails.TokenMint,
		FromAddress: txDetails.FromAddress,
		ToAddress:   intent.RecipientWallet,
		IntentID:    &intent.ID,
		BlockTime: func() *time.Time {
			if txResult.BlockTime != nil {
				t := time.Unix(int64(*txResult.BlockTime), 0)
				return &t
			}
			return nil
		}(),
		Slot: func() *int64 {
			if txResult.Slot > 0 {
				s := int64(txResult.Slot)
				return &s
			}
			return nil
		}(),
	}

	if _, err := r.State.DB.GetDB().NewInsert().Model(solanaTransaction).Exec(ctx); err != nil {
		log.WithError(err).Error("Failed to create SolanaTransaction record for Solana Pay intent")
	}

	return nil
}

// buildPaymentIntentResponse builds a PaymentIntentObject from an intent model
func buildPaymentIntentResponse(intent *models.SolanaPaymentIntent, r *Request) api.PaymentIntentObject {
	resp := api.PaymentIntentObject{
		ID:       api.FormatPaymentIntentID(intent.ID),
		Object:   "payment_intent",
		Status:   string(intent.Status),
		Amount:   intent.Amount,
		Currency: "usd",
		PaymentMethod: &api.PaymentIntentPaymentMethod{
			Type:      "solana",
			Token:     intent.Token,
			TokenMint: intent.TokenMint,
		},
		Created: intent.CreatedAt.Unix(),
	}

	if intent.ExpiresAt != nil {
		resp.ExpiresAt = intent.ExpiresAt.Unix()
	}

	if intent.PayerWallet != nil {
		resp.PaymentMethod.Wallet = *intent.PayerWallet
	}

	// Add token amount if we can compute it
	decimals := getTokenDecimals(r, intent.Token)
	resp.PaymentMethod.TokenAmount = formatTokenAmountFromUnits(intent.ExpectedAmountLamports, decimals)

	// Add transaction info based on flow type
	resp.Transaction = &api.PaymentIntentTransaction{}
	if intent.Reference != nil {
		resp.Transaction.Reference = *intent.Reference
	}
	if intent.TransactionSignature != nil {
		resp.Transaction.Signature = *intent.TransactionSignature
	}

	return resp
}

// getTokenDecimals returns the decimals for a token symbol
func getTokenDecimals(r *Request, token string) int {
	if r.State.Config.Solana != nil {
		if tokenCfg, ok := r.State.Config.Solana.SupportedTokens[token]; ok {
			return tokenCfg.Decimals
		}
	}
	// Default to 9 for SOL
	if strings.ToUpper(token) == "SOL" {
		return 9
	}
	// Default to 6 for stablecoins
	return 6
}

// formatTokenAmountFromUnits converts raw units to a human-readable string
func formatTokenAmountFromUnits(units uint64, decimals int) string {
	return formatTokenAmountPI(units, decimals)
}

// formatTokenAmountPI formats token amount (avoid name collision with solana_qr.go)
func formatTokenAmountPI(units uint64, decimals int) string {
	if decimals <= 0 {
		return fmt.Sprintf("%d", units)
	}
	numerator := new(big.Int).SetUint64(units)
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	return new(big.Rat).SetFrac(numerator, denominator).FloatString(decimals)
}

// generateQRReference generates a unique reference for QR payments (kept for reference validation)
func generateQRReference(userID string, priceID uuid.UUID, timestamp int64) string {
	referenceData := fmt.Sprintf("%s:%s:%d", userID, priceID.String(), timestamp)
	referenceHash := sha256.Sum256([]byte(referenceData))
	return base64.URLEncoding.EncodeToString(referenceHash[:])
}
