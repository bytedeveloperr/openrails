package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/database"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/integrations/mobius"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/supabase-community/gotrue-go/types"
)

type SubscribeData struct {
	Email        string `json:"email"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Address1     string `json:"address1"`
	City         string `json:"city"`
	State        string `json:"state"`
	Zip          string `json:"zip"`
	Country      string `json:"country"`
	PriceID      string `json:"price_id"` // Wave 18: Use PriceID instead of PlanID
	Processor    string `json:"processor"`
	PaymentToken string `json:"payment_token"` // CollectJS payment token for Mobius
}

type SubscriptionService struct {
	SubscriptionRepo      *repo.SubscriptionRepo
	ProductRepo           *repo.ProductRepo
	PriceRepo             *repo.PriceRepo
	PurchaseRepo          *repo.PurchaseRepo
	UserRoleGrantRepo     *repo.UserRoleGrantRepo
	NotificationQueueRepo *repo.NotificationQueueRepo
	MobiusPaymentRepo     *repo.MobiusPaymentMethodRepo
	CCBillRESTClient      *ccbill.RESTClient
	MobiusClient          *mobius.MobiusClient
	DB                    *database.DB
}

type PaymentProcessor = int

const (
	ProcessorCCBill = "ccbill"
	ProcessorMobius = "mobius"

	CurrencyUSD = "USD"
	CurrencyEUR = "EUR"

	BillingCycleMonthly = 30

	WebhookSourceCCBill = "ccbill_webhook"
	WebhookSourceMobius = "mobius_webhook"
	WebhookSourceSystem = "system"

	EventReasonSubscriptionExpired        = "subscription_expired"
	EventReasonSubscriptionDeletedWebhook = "subscription_deleted_via_webhook"
	EventReasonPaymentDeclined            = "payment_declined"

	StatusMessagePending = "pending"
)

const (
	CCBill PaymentProcessor = iota
	Mobius
)

var (
	PaymentProcessors = map[string]PaymentProcessor{
		ProcessorCCBill: CCBill,
		ProcessorMobius: Mobius,
	}
)

type SubscribeResponse struct {
	URL            string `json:"url,omitempty"`
	Status         string `json:"status,omitempty"`
	Message        string `json:"message,omitempty"`
	SubscriptionID string `json:"subscription_id,omitempty"`
}

func (s *SubscriptionService) Subscribe(ctx context.Context, data *SubscribeData, user *types.User) (any, error) {
	// Get price and product information
	priceID, err := uuid.Parse(data.PriceID)
	if err != nil {
		return nil, fmt.Errorf("invalid price ID: %w", err)
	}

	price, err := s.PriceRepo.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("price not found: %w", err)
	}

	// Check for existing active subscription
	existingSub, err := s.SubscriptionRepo.GetByUserID(ctx, user.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing subscription: %w", err)
	}

	if existingSub != nil && existingSub.Status == models.StatusActive {
		return nil, errors.New("user already has an active subscription")
	}

	processor := data.Processor
	if processor == "" {
		processor = ProcessorCCBill
	}

	switch processor {
	case ProcessorCCBill:
		// CCBill now uses FlexForm integration instead of direct payment processing
		return map[string]any{
			"status":  "redirect_required",
			"message": "CCBill payments now use FlexForm integration",
			"instructions": map[string]string{
				"step1": "Generate FlexForm URL using POST /api/v1/subscriptions/ccbill/flexform-url",
				"step2": "Embed the returned iframe_url in your frontend",
				"step3": "User completes payment in the embedded CCBill form",
				"step4": "Subscription will be activated via webhook upon successful payment",
			},
			"flexform_endpoint": "/api/v1/subscriptions/ccbill/flexform-url",
		}, nil
	case ProcessorMobius:
		// Use payment token flow for CollectJS integration
		params := mobius.RecurringPaymentData{
			CardUserData: mobius.CardUserData{
				FirstName: data.FirstName,
				LastName:  data.LastName,
				Address1:  data.Address1,
				City:      data.City,
				State:     data.State,
				Zip:       data.Zip,
				Country:   data.Country,
			},
			PlanID:       *price.MobiusPlanID, // Use price's Mobius plan ID
			Amount:       price.Amount,        // Use price amount
			Currency:     price.Currency,      // Use price currency
			Email:        user.Email,
			PaymentToken: data.PaymentToken, // CollectJS payment token
		}

		resp, err := s.MobiusClient.AddRecurringSubscription(params)
		if err != nil {
			return nil, err
		}

		return resp, nil
	default:
		return nil, errors.New("invalid payment processor")
	}
}

// ensureSubscription creates or gets existing subscription for Wave 18
func (s *SubscriptionService) ensureSubscription(ctx context.Context, user *types.User, price *models.Price) (*models.Subscription, error) {
	subscription, err := s.SubscriptionRepo.GetByUserID(ctx, user.ID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("an error occurred while retrieving subscription")
		}

		// Create new subscription with Wave 18 schema
		subscription = &models.Subscription{
			ID:                      uuid.New(),
			UserID:                  user.ID,
			PriceID:                 price.ID,
			Status:                  models.StatusPending,
			StartedAt:               time.Now(),             // Will be updated when subscription becomes active
			Processor:               models.ProcessorCCBill, // Default processor
			ProcessorSubscriptionID: "",                     // Will be set by payment processor
		}
		if err := s.SubscriptionRepo.Create(ctx, subscription); err != nil {
			return nil, err
		}
	}

	return subscription, nil
}

// GetUserSubscription retrieves the current subscription for a user
func (s *SubscriptionService) GetUserSubscription(ctx context.Context, userID uuid.UUID) (*models.Subscription, error) {
	return s.SubscriptionRepo.GetByUserID(ctx, userID)
}

// CancelUserSubscription cancels a user's subscription
func (s *SubscriptionService) CancelUserSubscription(ctx context.Context, userID uuid.UUID, feedback string) error {
	subscription, err := s.SubscriptionRepo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	if subscription.Status != models.StatusActive {
		return errors.New("subscription is not active")
	}

	now := time.Now()
	cancelType := models.CancelTypeUser
	subscription.Status = models.StatusCancelled
	subscription.CancelledAt = &now
	subscription.CancelType = &cancelType
	if feedback != "" {
		subscription.CancelFeedback = &feedback
	}

	if err := s.SubscriptionRepo.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Revoke role grants
	if err := s.UserRoleGrantRepo.RevokeBySubSourceID(ctx, subscription.ID); err != nil {
		log.WithError(err).Error("failed to revoke role grants for cancelled subscription")
	}

	// Add notification
	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    userID,
		EventType: models.NotificationPremiumEnded,
	}
	if err := s.NotificationQueueRepo.Create(ctx, notification); err != nil {
		log.WithError(err).Error("failed to create cancellation notification")
	}

	return nil
}

// GetAvailableProducts returns all active products with their prices
func (s *SubscriptionService) GetAvailableProducts(ctx context.Context) ([]*models.Product, error) {
	products, err := s.ProductRepo.GetActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active products: %w", err)
	}

	// Load prices for each product
	for _, product := range products {
		prices, err := s.PriceRepo.GetActiveByProductID(ctx, product.ID)
		if err != nil {
			log.WithFields(log.Fields{
				"product_id": product.ID,
				"error":      err.Error(),
			}).Warn("Failed to load prices for product")
			continue
		}
		product.Prices = prices
	}

	return products, nil
}
