package handlers

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/message"
)

// ProcessWebhook godoc
// @Summary Process webhook from payment processors
// @Schemes
// @Description Processes webhooks from Mobius and CCBill payment processors
// @Tags billing-webhooks
// @Accept json
// @Produce json
// @Param processor path string true "Processor name" Enums(mobius,ccbill)
// @Success 200 {object} handlers.ProcessWebhookResponse
// @Failure 400 {object} message.MessageResponse "Bad request"
// @Failure 401 {object} message.MessageResponse "Unauthorized - Invalid signature"
// @Failure 500 {object} message.MessageResponse "Internal server error"
// @Router /billing/webhooks/{processor} [post]
func (h *WebhookHandler) ProcessWebhook(c *gin.Context) {
	processor := c.Param("processor")
	if processor == "" {
		c.JSON(http.StatusBadRequest, message.Message("Processor parameter is required"))
		return
	}

	if processor != "mobius" && processor != "ccbill" {
		c.JSON(http.StatusBadRequest, message.Message("Invalid processor. Supported: mobius, ccbill"))
		return
	}

	// Read the raw request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.WithError(err).Error("Failed to read webhook body")
		c.JSON(http.StatusBadRequest, message.Message("Failed to read request body"))
		return
	}

	// Extract headers for signature verification
	headers := make(map[string]string)
	for name, values := range c.Request.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}

	// Get client IP address for verification
	ipAddress := c.ClientIP()

	// Process the webhook
	result, err := h.webhookService.ProcessWebhook(c.Request.Context(), processor, body, headers, ipAddress)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"processor":  processor,
			"ip_address": ipAddress,
		}).Error("Failed to process webhook")

		// Check if it's a signature verification error
		if err == services.ErrInvalidSignature {
			c.JSON(http.StatusUnauthorized, message.Message("Invalid webhook signature"))
			return
		}

		// Check if it's an IP verification error
		if err == services.ErrUnauthorizedIP {
			c.JSON(http.StatusUnauthorized, message.Message("Unauthorized IP address"))
			return
		}

		c.JSON(http.StatusInternalServerError, message.Message("Failed to process webhook"))
		return
	}

	response := NewProcessWebhookResponse(result)
	c.JSON(http.StatusOK, response)
}
