package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/services"
	ipverify "github.com/doujins-org/doujins-billing/internal/utils"
)

func Webhook(r *Request) {
	processor := r.Param("processor")
	provider := strings.Trim(strings.Trim(r.Param("provider"), "/"), " ")
	if provider == "" {
		provider = strings.TrimSpace(r.Query("provider"))
	}

	// NOTE: Webhook authentication can be bypassed for testing by setting test_mode: true
	// in the respective processor config (nmi.test_mode or ccbill.test_mode)
	// This is useful for integration tests and development environments

	// Create dead letter service
	deadLetterService := &services.DeadLetterService{
		DB:                       r.State.DB,
		NotificationQueueService: r.State.NotificationQueueService,
	}

	// Capture request headers for dead letter logging
	headers := make(map[string]string)
	for key, values := range r.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	clientIP := r.GetClientIP()

	log.WithFields(log.Fields{
		"processor": processor,
		"client_ip": clientIP,
	}).Debug("Received webhook")

	ccbillTestMode := true
	if r.State != nil && r.State.CCBillRESTClient != nil {
		if cfg := r.State.CCBillRESTClient.Config(); cfg != nil {
			ccbillTestMode = cfg.TestMode
		}
	}

	switch processor {
	case services.ProcessorCCBill:
		// Check if CCBill is in test mode - bypass authentication for testing
		if !ccbillTestMode {
			// Verify CCBill webhook comes from authorized IP ranges
			if !ipverify.IsValidCCBillIP(clientIP) {
				log.WithFields(log.Fields{
					"client_ip":  clientIP,
					"processor":  "ccbill",
					"event_type": r.Query("eventType"),
				}).Warn("CCBill webhook rejected - unauthorized IP address")

				// Log the security violation for monitoring
				deadLetterService.LogInvalidPayload(
					context.Background(),
					"ccbill",
					[]byte(fmt.Sprintf("Unauthorized IP: %s", clientIP)),
					fmt.Errorf("webhook from unauthorized IP address: %s", clientIP),
					headers,
					clientIP,
				)

				r.ErrorJSON(http.StatusForbidden, "Unauthorized webhook source")
				return
			}

			log.WithField("client_ip", clientIP).Debug("CCBill webhook authenticated - valid IP range")
		} else {
			log.WithField("client_ip", clientIP).Debug("CCBill webhook authentication bypassed - test mode enabled")
		}

		body, err := readRequestBody(r.Request.Body)
		if err != nil {
			deadLetterService.LogInvalidPayload(context.Background(), "ccbill", nil, err, headers, clientIP)
			r.ErrorJSON(http.StatusInternalServerError, "Failed to read request body")
			return
		}

		eventType := r.Query("eventType")
		data := services.CCBillWebhookEvent{
			EventType: eventType,
			EventBody: body,
		}

		service := services.CCBillWebhookService{
			Data:                         data,
			DB:                           r.State.DB,
			PriceService:                 r.State.PriceService,
			ProductService:               r.State.ProductService,
			CCBillClient:                 r.State.CCBillRESTClient,
			BillingEventService:          r.State.BillingEventService,
			SubscriptionService:          r.State.SubscriptionService,
			NotificationQueueService:     r.State.NotificationQueueService,
			NotificationService:          r.State.NotificationService,
			SubscriptionLifecycleService: r.State.SubscriptionLifecycleService,
		}

		if err := service.HandleCCBillWebhook(context.Background()); err != nil {
			log.WithError(err).Error("failed to process CCBill webhook")
			r.ErrorJSON(http.StatusInternalServerError, err.Error())
			return
		}
	case services.ProcessorNMI:
		handleNMIWebhook(r, provider)
		return
	default:
		webhookBody, readErr := readRequestBody(r.Request.Body)
		if readErr != nil {
			log.WithError(readErr).WithField("processor", processor).Warn("Failed to read body for unknown webhook processor")
		}
		// Log unknown processor to dead letter queue
		deadLetterService.LogUnknownEvent(context.Background(), processor, "unknown", json.RawMessage(webhookBody), headers, clientIP)
		r.ErrorJSON(http.StatusBadRequest, "Invalid processor")
		return
	}

}

func handleNMIWebhook(r *Request, provider string) {
	// Read the request body for signature verification
	body, err := readRequestBody(r.Request.Body)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, "Failed to read request body")
		return
	}

	providerKey := strings.TrimSpace(strings.ToLower(provider))
	if providerKey == "" {
		providerKey = "mobius"
	}

	client, ok := r.State.NMIClients[providerKey]
	if !ok || client == nil {
		r.ErrorJSON(http.StatusNotFound, fmt.Sprintf("unknown nmi provider '%s'", providerKey))
		return
	}

	// Check if NMI is in test mode - bypass authentication for testing
	if !client.Config().TestMode {
		// Verify webhook signature if webhook secret is configured
		if client.GetWebhookSecret() != "" {
			signature := r.Request.Header.Get("X-Signature")
			if signature == "" {
				signature = r.Request.Header.Get("X-NMI-Signature")
			}
			if signature == "" {
				signature = r.Request.Header.Get("X-Mobius-Signature")
			}
			if signature == "" {
				log.Error("Missing webhook signature for NMI webhook")
				r.ErrorJSON(http.StatusUnauthorized, "Missing webhook signature")
				return
			}

			if err := client.VerifyWebhookSignature(body, signature); err != nil {
				log.WithError(err).Error("NMI webhook signature verification failed")
				r.ErrorJSON(http.StatusUnauthorized, "Invalid webhook signature")
				return
			}
		} else {
			log.Warn("NMI webhook secret not configured - skipping signature verification")
		}
	} else {
		log.Debug("NMI webhook authentication bypassed - test mode enabled")
	}

	var data services.NMIWebhookEvent
	if err := json.Unmarshal(body, &data); err != nil {
		log.WithError(err).Error("failed to parse NMI webhook JSON")
		r.ErrorJSON(http.StatusBadRequest, "Invalid JSON data")
		return
	}
	service := services.NMIWebhookService{
		Data:                         data,
		DB:                           r.State.DB,
		PriceService:                 r.State.PriceService,
		ProductService:               r.State.ProductService,
		Provider:                     providerKey,
		NMIClient:                    client,
		BillingEventService:          r.State.BillingEventService,
		SubscriptionService:          r.State.SubscriptionService,
		NotificationQueueService:     r.State.NotificationQueueService,
		SubscriptionLifecycleService: r.State.SubscriptionLifecycleService,
	}

	if err := service.HandleNMIWebhook(context.Background()); err != nil {
		log.WithError(err).Error("failed to process webhook")
		r.ErrorJSON(http.StatusInternalServerError, err.Error())
		return
	}
}

func readRequestBody(body io.ReadCloser) ([]byte, error) {
	if body == nil {
		return []byte{}, nil
	}
	defer body.Close()
	return io.ReadAll(body)
}
