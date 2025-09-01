package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/middleware"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/message"
)

// GetSupportedTokens godoc
// @Summary Get supported Solana tokens
// @Schemes
// @Description Retrieves list of supported tokens for Solana payments
// @Tags billing-solana
// @Accept json
// @Produce json
// @Success 200 {object} handlers.GetSupportedTokensResponse
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/solana/tokens [get]
func (h *SolanaHandler) GetSupportedTokens(c *gin.Context) {
	tokens, err := h.solanaService.GetSupportedTokens(c.Request.Context())
	if err != nil {
		log.WithError(err).Error("Failed to get supported tokens")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to retrieve supported tokens"))
		return
	}

	response := NewGetSupportedTokensResponse(tokens)
	c.JSON(http.StatusOK, response)
}

// GenerateQR godoc
// @Summary Generate Solana Pay QR code
// @Schemes
// @Description Generates a Solana Pay QR code for payment
// @Tags billing-solana
// @Accept json
// @Produce json
// @Param req body handlers.GenerateQRBodyParams true "QR generation data"
// @Success 200 {object} handlers.GenerateQRResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/solana/qr [post]
func (h *SolanaHandler) GenerateQR(c *gin.Context) {
	var req GenerateQRRequest
	err := c.ShouldBindQuery(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind generate QR request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithError(err).Error("Invalid price ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid price ID format"))
		return
	}

	serviceRequest := &services.QRCodeRequest{
		PriceID:     priceID,
		TokenSymbol: req.TokenSymbol,
		UserWallet:  req.UserWallet,
	}

	result, err := h.solanaService.GenerateQRCode(c.Request.Context(), serviceRequest)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":  userCtx.User.ID,
			"price_id": req.PriceID,
			"token":    req.TokenSymbol,
		}).Error("Failed to generate QR code")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to generate QR code"))
		return
	}

	response := NewGenerateQRResponse(result)
	c.JSON(http.StatusOK, response)
}

// GenerateTransaction godoc
// @Summary Generate Solana transaction
// @Schemes
// @Description Generates a Solana transaction for payment
// @Tags billing-solana
// @Accept json
// @Produce json
// @Param req body handlers.GenerateTransactionBodyParams true "Transaction generation data"
// @Success 200 {object} handlers.GenerateTransactionResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/solana/transaction [post]
func (h *SolanaHandler) GenerateTransaction(c *gin.Context) {
	var req GenerateTransactionRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind generate transaction request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	priceID, err := uuid.Parse(req.PriceID)
	if err != nil {
		log.WithError(err).Error("Invalid price ID format")
		c.JSON(http.StatusBadRequest, message.Message("Invalid price ID format"))
		return
	}

	serviceRequest := &services.TransactionRequest{
		PriceID:     priceID,
		TokenSymbol: req.TokenSymbol,
		Account:     req.Account,
	}

	result, err := h.solanaService.GenerateTransaction(c.Request.Context(), serviceRequest)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":  userCtx.User.ID,
			"price_id": req.PriceID,
			"token":    req.TokenSymbol,
			"account":  req.Account,
		}).Error("Failed to generate transaction")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to generate transaction"))
		return
	}

	response := NewGenerateTransactionResponse(result)
	c.JSON(http.StatusOK, response)
}

// SubmitTransaction godoc
// @Summary Submit Solana transaction
// @Schemes
// @Description Submits a signed Solana transaction for processing
// @Tags billing-solana
// @Accept json
// @Produce json
// @Param req body handlers.SubmitTransactionBodyParams true "Transaction submission data"
// @Success 200 {object} handlers.SubmitTransactionResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/solana/submit [post]
func (h *SolanaHandler) SubmitTransaction(c *gin.Context) {
	var req SubmitTransactionRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		log.WithError(err).Error("Failed to bind submit transaction request")
		c.JSON(http.StatusBadRequest, message.Message("Invalid request"))
		return
	}

	userCtx := middleware.GetUserContext(c.Request.Context())
	if userCtx.User == nil {
		c.JSON(http.StatusUnauthorized, message.Message("User authentication required"))
		return
	}

	serviceRequest := &services.TransactionSubmissionRequest{
		Signature: req.Signature,
	}

	result, err := h.solanaService.SubmitTransaction(c.Request.Context(), serviceRequest, userCtx.User)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":   userCtx.User.ID,
			"signature": req.Signature,
		}).Error("Failed to submit transaction")
		c.JSON(http.StatusInternalServerError, message.Message("Failed to submit transaction"))
		return
	}

	response := NewSubmitTransactionResponse(result)
	c.JSON(http.StatusOK, response)
}
