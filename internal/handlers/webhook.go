package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/services"
	ipverify "github.com/doujins-org/doujins-billing/internal/utils"
)

func Webhook(r *Request) {
	processor := r.Param("processor")

	// NOTE: Webhook authentication can be bypassed for testing by setting test_mode: true
	// in the respective processor config (mobius.test_mode or ccbill.test_mode)
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

	fmt.Println("Received webhook request for processor:", processor)
	fmt.Println("Client IP:", clientIP)

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
	case services.ProcessorMobius:
		handleMobiusWebhook(r)
		return
	default:
		// Log unknown processor to dead letter queue
		body, readErr := readRequestBody(r.Request.Body)
		if readErr == nil {
			deadLetterService.LogUnknownEvent(context.Background(), processor, "unknown", json.RawMessage(body), headers, clientIP)
		}
		r.ErrorJSON(http.StatusBadRequest, "Invalid processor")
		return
	}

}

func handleMobiusWebhook(r *Request) {
	// Read the request body for signature verification
	body, err := readRequestBody(r.Request.Body)
	if err != nil {
		r.ErrorJSON(http.StatusInternalServerError, "Failed to read request body")
		return
	}

	// Check if Mobius is in test mode - bypass authentication for testing
	if !r.State.MobiusClient.Config().TestMode {
		// Verify webhook signature if webhook secret is configured
		if r.State.MobiusClient.GetWebhookSecret() != "" {
			signature := r.Request.Header.Get("X-Signature")
			if signature == "" {
				signature = r.Request.Header.Get("X-Mobius-Signature")
			}
			if signature == "" {
				log.Error("Missing webhook signature for Mobius webhook")
				r.ErrorJSON(http.StatusUnauthorized, "Missing webhook signature")
				return
			}

			if err := r.State.MobiusClient.VerifyWebhookSignature(body, signature); err != nil {
				log.WithError(err).Error("Mobius webhook signature verification failed")
				r.ErrorJSON(http.StatusUnauthorized, "Invalid webhook signature")
				return
			}
		} else {
			log.Warn("Mobius webhook secret not configured - skipping signature verification")
		}
	} else {
		log.Debug("Mobius webhook authentication bypassed - test mode enabled")
	}

	var data services.MobiusWebhookEvent
	if err := json.Unmarshal(body, &data); err != nil {
		log.WithError(err).Error("failed to parse Mobius webhook JSON")
		r.ErrorJSON(http.StatusBadRequest, "Invalid JSON data")
		return
	}
	service := services.MobiusWebhookService{
		Data:                         data,
		DB:                           r.State.DB,
		PriceService:                 r.State.PriceService,
		ProductService:               r.State.ProductService,
		MobiusClient:                 r.State.MobiusClient,
		BillingEventService:          r.State.BillingEventService,
		SubscriptionService:          r.State.SubscriptionService,
		NotificationQueueService:     r.State.NotificationQueueService,
		SubscriptionLifecycleService: r.State.SubscriptionLifecycleService,
	}

	if err := service.HandleMobiusWebhook(context.Background()); err != nil {
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
