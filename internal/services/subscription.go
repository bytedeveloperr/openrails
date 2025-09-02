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
	"github.com/doujins-org/doujins-billing/pkg/query"
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
	PriceID      string `json:"price_id"`
	Processor    string `json:"processor"`
	PaymentToken string `json:"payment_token"`
}

type GetSubscriptionsFilters struct {
	UserID    uuid.UUID `form:"user_id"`
	Status    string    `form:"status"`
	PriceID   uuid.UUID `form:"price_id"`
	Processor string    `form:"processor"`
}

type SubscriptionService struct {
	DB                       *db.DB
	PriceService             *PriceService
	ProductService           *ProductService
	UserRoleGrantService     *UserRoleGrantService
	NotificationQueueService *NotificationQueueService
	CCBillRESTClient         *ccbill.RESTClient
	MobiusClient             *mobius.MobiusClient
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

func (s *SubscriptionService) Subscribe(ctx context.Context, data *SubscribeData, user *models.User) (any, error) {
	// Get price and product information
	priceID, err := uuid.Parse(data.PriceID)
	if err != nil {
		return nil, fmt.Errorf("invalid price ID: %w", err)
	}

	price, err := s.PriceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("price not found: %w", err)
	}

	// Check for existing active subscription
	existingSub, err := s.GetByUserID(ctx, user.ID)
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
			Email:        *user.Email,
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
	subscription, err := s.GetByUserID(ctx, user.ID)
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
		if err := s.Create(ctx, subscription); err != nil {
			return nil, err
		}
	}

	return subscription, nil
}

// GetUserSubscription retrieves the current subscription for a user
func (s *SubscriptionService) GetUserSubscription(ctx context.Context, userID uuid.UUID) (*models.Subscription, error) {
	return s.GetByUserID(ctx, userID)
}

// CancelUserSubscription cancels a user's subscription
func (s *SubscriptionService) CancelUserSubscription(ctx context.Context, userID uuid.UUID, feedback string) error {
	subscription, err := s.GetByUserID(ctx, userID)
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

	if err := s.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Revoke role grants
	if err := s.UserRoleGrantService.RevokeBySubSourceID(ctx, subscription.ID); err != nil {
		log.WithError(err).Error("failed to revoke role grants for cancelled subscription")
	}

	// Add notification
	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    userID,
		EventType: models.NotificationPremiumEnded,
	}
	if err := s.NotificationQueueService.Create(ctx, notification); err != nil {
		log.WithError(err).Error("failed to create cancellation notification")
	}

	return nil
}

// GetAvailableProducts returns all active products with their prices
func (s *SubscriptionService) GetAvailableProducts(ctx context.Context) ([]*models.Product, error) {
	products, err := s.ProductService.GetActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active products: %w", err)
	}

	// Load prices for each product
	for _, product := range products {
		prices, err := s.PriceService.GetActiveByProductID(ctx, product.ID)
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

func NewSubscriptionService(db *db.DB) *SubscriptionService {
	return &SubscriptionService{DB: db}
}

func (s *SubscriptionService) GetDB() *db.DB {
	return s.DB
}

func (s *SubscriptionService) Create(ctx context.Context, subscription *models.Subscription) error {
	result, err := s.DB.GetDB().NewInsert().Model(subscription).Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

func (s *SubscriptionService) GetByID(ctx context.Context, id uuid.UUID) (*models.Subscription, error) {
	var subscription models.Subscription
	err := s.DB.GetDB().NewSelect().Model(&subscription).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

func (s *SubscriptionService) GetByUserID(ctx context.Context, id uuid.UUID) (*models.Subscription, error) {
	var subscription models.Subscription
	err := s.DB.GetDB().NewSelect().Model(&subscription).Relation("Price").Where("user_id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}

	return &subscription, nil
}

func (s *SubscriptionService) GetByUserIDAndPriceID(ctx context.Context, id uuid.UUID, priceID uuid.UUID) (*models.Subscription, error) {
	var subscription models.Subscription
	if err := s.DB.GetDB().NewSelect().
		Model(&subscription).
		Where("user_id = ?", id).
		Where("price_id = ?", priceID).
		Scan(ctx); err != nil {
		return nil, err
	}

	return &subscription, nil
}

func (s *SubscriptionService) Update(ctx context.Context, subscription *models.Subscription) error {
	result, err := s.DB.GetDB().NewUpdate().Model(subscription).WherePK().Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected < 1 {
		return errors.New("no rows affected")
	}

	return nil
}

func (s *SubscriptionService) GetSubscribers(ctx context.Context, params query.QueryOptions[GetSubscriptionsFilters]) ([]*models.Subscription, int64, error) {
	q := s.DB.GetDB().NewSelect().
		Model((*models.Subscription)(nil))
		// Note: User relationship is not preloaded - fetch separately if needed using UsertService.GetFullUser

	if params.Filters.Status != "" {
		q = q.Where("status = ?", params.Filters.Status)
	}

	if params.Filters.PriceID != uuid.Nil {
		q = q.Where("price_id = ?", params.Filters.PriceID)
	}

	if params.Filters.Processor != "" {
		q = q.Where("processor = ?", params.Filters.Processor)
	}

	var total int64
	count, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	total = int64(count)

	if params.Limit > 0 {
		q = q.Limit(params.Limit)
	}

	if params.Offset > 0 {
		q = q.Offset(params.Offset)
	}

	q = q.Order("created_at DESC")

	var subscriptions []*models.Subscription
	if err := q.Scan(ctx, &subscriptions); err != nil {
		return nil, 0, err
	}

	return subscriptions, total, nil
}

func (s *SubscriptionService) GetPaginatedByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]models.Subscription, int, error) {
	var subscriptions []models.Subscription
	var count int

	offset := (page - 1) * pageSize

	count, err := s.DB.GetDB().NewSelect().
		Model(&models.Subscription{}).
		Where("user_id = ?", userID).
		Count(ctx)

	if err != nil {
		return nil, 0, err
	}

	query := s.DB.GetDB().NewSelect().
		Model(&subscriptions).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset)

	if err := query.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return subscriptions, count, nil
}

// GetSubscriptionsWithDetailsForUser retrieves subscriptions with related price information for billing history
func (s *SubscriptionService) GetSubscriptionsWithDetailsForUser(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]models.Subscription, int, error) {
	var subscriptions []models.Subscription
	var count int

	offset := (page - 1) * pageSize

	// Get count
	count, err := s.DB.GetDB().NewSelect().
		Model(&models.Subscription{}).
		Where("user_id = ?", userID).
		Count(ctx)

	if err != nil {
		return nil, 0, err
	}

	// Get subscriptions with related data
	query := s.DB.GetDB().NewSelect().
		Model(&subscriptions).
		Relation("Price").
		Relation("PaymentMethod").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset)

	if err := query.Scan(ctx); err != nil {
		return nil, 0, err
	}

	return subscriptions, count, nil
}

// GetActiveSubscriptionsByUserID retrieves only active subscriptions for a user
func (s *SubscriptionService) GetActiveSubscriptionsByUserID(ctx context.Context, userID uuid.UUID) ([]models.Subscription, error) {
	var subscriptions []models.Subscription

	query := s.DB.GetDB().NewSelect().
		Model(&subscriptions).
		Relation("Price").
		Relation("PaymentMethod").
		Where("user_id = ? AND status = ?", userID, models.StatusActive).
		Order("created_at DESC")

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}

	return subscriptions, nil
}

// GetSubscriptionsByProcessorAndUserID retrieves subscriptions filtered by processor
func (s *SubscriptionService) GetSubscriptionsByProcessorAndUserID(ctx context.Context, userID uuid.UUID, processor models.Processor) ([]models.Subscription, error) {
	var subscriptions []models.Subscription

	query := s.DB.GetDB().NewSelect().
		Model(&subscriptions).
		Relation("Price").
		Relation("PaymentMethod").
		Where("user_id = ? AND processor = ?", userID, processor).
		Order("created_at DESC")

	if err := query.Scan(ctx); err != nil {
		return nil, err
	}

	return subscriptions, nil
}

// GetActiveSubscription retrieves the active subscription for a user
func (s *SubscriptionService) GetActiveSubscription(ctx context.Context, userID uuid.UUID) (*models.Subscription, error) {
	var subscription models.Subscription
	err := s.DB.GetDB().NewSelect().
		Model(&subscription).
		Where("user_id = ?", userID).
		Where("status = ?", models.StatusActive).
		Where("(current_period_ends_at IS NULL OR current_period_ends_at > NOW())").
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)

	if err != nil {
		return nil, err
	}

	return &subscription, nil
}

// GetByProcessorSubscriptionID finds a subscription by processor and processor_subscription_id
func (s *SubscriptionService) GetByProcessorSubscriptionID(ctx context.Context, processor, processorSubscriptionID string) (*models.Subscription, error) {
	var subscription models.Subscription
	err := s.DB.GetDB().NewSelect().
		Model(&subscription).
		Relation("Price").
		Where("processor = ?", processor).
		Where("processor_subscription_id = ?", processorSubscriptionID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// GetActiveSubscriptionsByProcessor gets all active subscriptions for a processor
func (s *SubscriptionService) GetActiveSubscriptionsByProcessor(ctx context.Context, processor string) ([]*models.Subscription, error) {
	var subscriptions []*models.Subscription
	err := s.DB.GetDB().NewSelect().
		Model(&subscriptions).
		Where("processor = ?", processor).
		Where("status = ?", "active").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return subscriptions, nil
}

// Delete removes a subscription from the database permanently
func (s *SubscriptionService) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := s.DB.GetDB().NewDelete().
		Model((*models.Subscription)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected < 1 {
		return errors.New("no rows affected")
	}

	return nil
}
