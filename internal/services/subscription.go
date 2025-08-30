package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
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
	CCBillRESTClient *ccbill.RESTClient
	MobiusClient     *mobius.MobiusClient
	DB               *db.DB
}

// GetPriceByID retrieves a price by its ID
func (s *SubscriptionService) GetPriceByID(ctx context.Context, id uuid.UUID) (*models.Price, error) {
	var price models.Price
	err := s.DB.GetDB().NewSelect().
		Model(&price).
		Where("id = ?", id).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get price by ID: %w", err)
	}
	return &price, nil
}

// GetSubscriptionByUserID retrieves a subscription by user ID
func (s *SubscriptionService) GetSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (*models.Subscription, error) {
	var subscription models.Subscription
	err := s.DB.GetDB().NewSelect().
		Model(&subscription).
		Where("user_id = ?", userID).
		Where("status = ?", models.StatusActive).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription by user ID: %w", err)
	}
	return &subscription, nil
}

// CreateSubscription creates a new subscription
func (s *SubscriptionService) CreateSubscription(ctx context.Context, subscription *models.Subscription) error {
	_, err := s.DB.GetDB().NewInsert().
		Model(subscription).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}
	return nil
}

// UpdateSubscription updates a subscription
func (s *SubscriptionService) UpdateSubscription(ctx context.Context, subscription *models.Subscription) error {
	_, err := s.DB.GetDB().NewUpdate().
		Model(subscription).
		Where("id = ?", subscription.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}
	return nil
}

// RevokeUserRolesBySubSourceID revokes user roles by subscription source ID
func (s *SubscriptionService) RevokeUserRolesBySubSourceID(ctx context.Context, subID uuid.UUID) error {
	_, err := s.DB.GetDB().NewUpdate().
		Model((*models.UserRoleGrant)(nil)).
		Set("revoked_at = ?", time.Now()).
		Where("sub_source_id = ?", subID).
		Where("revoked_at IS NULL").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke user roles: %w", err)
	}
	return nil
}

// CreateNotification creates a new notification
func (s *SubscriptionService) CreateNotification(ctx context.Context, notification *models.NotificationQueue) error {
	_, err := s.DB.GetDB().NewInsert().
		Model(notification).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}
	return nil
}

// GetActiveProducts retrieves all active products
func (s *SubscriptionService) GetActiveProducts(ctx context.Context) ([]*models.Product, error) {
	var products []*models.Product
	err := s.DB.GetDB().NewSelect().
		Model(&products).
		Where("active = ?", true).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active products: %w", err)
	}
	return products, nil
}

// GetActiveProductPrices retrieves active prices for a specific product
func (s *SubscriptionService) GetActiveProductPrices(ctx context.Context, productID uuid.UUID) ([]*models.Price, error) {
	var prices []*models.Price
	err := s.DB.GetDB().NewSelect().
		Model(&prices).
		Where("product_id = ?", productID).
		Where("active = ?", true).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active product prices: %w", err)
	}
	return prices, nil
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

	price, err := s.GetPriceByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("price not found: %w", err)
	}

	// Check for existing active subscription
	existingSub, err := s.GetSubscriptionByUserID(ctx, user.ID)
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
	subscription, err := s.GetSubscriptionByUserID(ctx, user.ID)
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
		if err := s.CreateSubscription(ctx, subscription); err != nil {
			return nil, err
		}
	}

	return subscription, nil
}

// GetUserSubscription retrieves the current subscription for a user
func (s *SubscriptionService) GetUserSubscription(ctx context.Context, userID uuid.UUID) (*models.Subscription, error) {
	return s.GetSubscriptionByUserID(ctx, userID)
}

// CancelUserSubscription cancels a user's subscription
func (s *SubscriptionService) CancelUserSubscription(ctx context.Context, userID uuid.UUID, feedback string) error {
	subscription, err := s.GetSubscriptionByUserID(ctx, userID)
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

	if err := s.UpdateSubscription(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Revoke role grants
	if err := s.RevokeUserRolesBySubSourceID(ctx, subscription.ID); err != nil {
		log.WithError(err).Error("failed to revoke role grants for cancelled subscription")
	}

	// Add notification
	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    userID,
		EventType: models.NotificationPremiumEnded,
	}
	if err := s.CreateNotification(ctx, notification); err != nil {
		log.WithError(err).Error("failed to create cancellation notification")
	}

	return nil
}

// GetAvailableProducts returns all active products with their prices
func (s *SubscriptionService) GetAvailableProducts(ctx context.Context) ([]*models.Product, error) {
	products, err := s.GetActiveProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active products: %w", err)
	}

	// Load prices for each product
	for _, product := range products {
		prices, err := s.GetActiveProductPrices(ctx, product.ID)
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
