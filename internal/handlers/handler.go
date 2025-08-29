package handlers

import (
	"github.com/doujins-org/doujins-billing/internal/services"
)

// Handler represents a generic handler with common functionality
type Handler struct {
	// Services (to be embedded by specific handlers)
}

// SubscriptionHandler handles subscription-related endpoints
type SubscriptionHandler struct {
	subscriptionService *services.SubscriptionService
	idempotencyService  *services.IdempotencyService
}

// NewSubscriptionHandler creates a new subscription handler
func NewSubscriptionHandler(subscriptionService *services.SubscriptionService, idempotencyService *services.IdempotencyService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subscriptionService: subscriptionService,
		idempotencyService:  idempotencyService,
	}
}

// PaymentMethodHandler handles payment method endpoints
type PaymentMethodHandler struct {
	paymentMethodService *services.PaymentMethodService
}

// NewPaymentMethodHandler creates a new payment method handler
func NewPaymentMethodHandler(paymentMethodService *services.PaymentMethodService) *PaymentMethodHandler {
	return &PaymentMethodHandler{
		paymentMethodService: paymentMethodService,
	}
}

// SolanaHandler handles Solana payment endpoints
type SolanaHandler struct {
	solanaService *services.SolanaService
}

// NewSolanaHandler creates a new Solana handler
func NewSolanaHandler(solanaService *services.SolanaService) *SolanaHandler {
	return &SolanaHandler{
		solanaService: solanaService,
	}
}

// WebhookHandler handles webhook endpoints
type WebhookHandler struct {
	webhookService *services.WebhookService
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(webhookService *services.WebhookService) *WebhookHandler {
	return &WebhookHandler{
		webhookService: webhookService,
	}
}

// AdminHandler handles admin-only endpoints
type AdminHandler struct {
	subscriptionService *services.SubscriptionService
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(subscriptionService *services.SubscriptionService) *AdminHandler {
	return &AdminHandler{
		subscriptionService: subscriptionService,
	}
}
