package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/integrations/nmi"
	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
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
	Provider     string `json:"provider,omitempty"`
	PaymentToken string `json:"payment_token"`
}

type GetSubscriptionsFilters struct {
	UserID    string    `form:"user_id"`
	Status    string    `form:"status"`
	PriceID   uuid.UUID `form:"price_id"`
	Processor string    `form:"processor"`
}

type SubscriptionService struct {
	subscriptionRepo         *repo.SubscriptionRepo
	PriceService             *PriceService
	ProductService           *ProductService
	NotificationQueueService *NotificationQueueService
	CCBillRESTClient         *ccbill.RESTClient
	NMIClients               map[string]*nmi.NMIClient
}

func (s *SubscriptionService) nmiClientForProvider(provider string) (*nmi.NMIClient, error) {
	providerKey := strings.TrimSpace(strings.ToLower(provider))
	if providerKey == "" {
		providerKey = "mobius"
	}
	if s.NMIClients == nil {
		return nil, fmt.Errorf("nmi provider '%s' is not configured", providerKey)
	}
	client, ok := s.NMIClients[providerKey]
	if !ok {
		return nil, fmt.Errorf("nmi provider '%s' is not configured", providerKey)
	}
	return client, nil
}

type PaymentProcessor = int

const (
	ProcessorCCBill = "ccbill"
	ProcessorNMI    = "nmi"

	CurrencyUSD = "USD"
	CurrencyEUR = "EUR"

	BillingCycleMonthly = 30

	WebhookSourceCCBill = "ccbill_webhook"
	WebhookSourceNMI    = "nmi_webhook"
	WebhookSourceSystem = "system"

	EventReasonSubscriptionExpired        = "subscription_expired"
	EventReasonSubscriptionDeletedWebhook = "subscription_deleted_via_webhook"
	EventReasonPaymentDeclined            = "payment_declined"

	StatusMessagePending = "pending"
)

const (
	CCBill PaymentProcessor = iota
	NMI
)

var (
	PaymentProcessors = map[string]PaymentProcessor{
		ProcessorCCBill: CCBill,
		ProcessorNMI:    NMI,
	}
)

type SubscribeResponse struct {
	URL            string `json:"url,omitempty"`
	Status         string `json:"status,omitempty"`
	Message        string `json:"message,omitempty"`
	SubscriptionID string `json:"subscription_id,omitempty"`
}

func (s *SubscriptionService) Subscribe(ctx context.Context, data *SubscribeData, user *UserIdentity) (any, error) {
	priceID, err := uuid.Parse(data.PriceID)
	if err != nil {
		return nil, fmt.Errorf("invalid price ID: %w", err)
	}

	price, err := s.PriceService.GetByID(ctx, priceID)
	if err != nil {
		return nil, fmt.Errorf("price not found: %w", err)
	}

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
				"step1": "Generate FlexForm URL using POST /v1/subscriptions/ccbill/flexform-url",
				"step2": "Embed the returned iframe_url in your frontend",
				"step3": "User completes payment in the embedded CCBill form",
				"step4": "Subscription will be activated via webhook upon successful payment",
			},
			"flexform_endpoint": "/v1/subscriptions/ccbill/flexform-url",
		}, nil
	case ProcessorNMI:
		provider := strings.TrimSpace(strings.ToLower(data.Provider))
		if price.NMIProvider != nil {
			expected := strings.TrimSpace(strings.ToLower(*price.NMIProvider))
			if provider != "" && provider != expected {
				return nil, fmt.Errorf("provider %s does not match price provider %s", provider, expected)
			}
			provider = expected
		}

		if provider == "" {
			provider = "mobius"
		}

		client, err := s.nmiClientForProvider(provider)
		if err != nil {
			return nil, err
		}

		if price.NMIPlanID == nil || strings.TrimSpace(*price.NMIPlanID) == "" {
			return nil, fmt.Errorf("price %s is missing an NMI plan configuration", price.ID)
		}

		email := strings.TrimSpace(data.Email)
		if user.Email != nil && strings.TrimSpace(*user.Email) != "" {
			email = strings.TrimSpace(*user.Email)
		}
		if email == "" {
			return nil, errors.New("email is required to create a subscription")
		}

		subscriptionID := uuid.New()
		params := nmi.RecurringPaymentData{
			CardUserData: nmi.CardUserData{
				FirstName: data.FirstName,
				LastName:  data.LastName,
				Address1:  data.Address1,
				City:      data.City,
				State:     data.State,
				Zip:       data.Zip,
				Country:   data.Country,
			},
			PlanID:       strings.TrimSpace(*price.NMIPlanID),
			Amount:       price.Amount,
			Currency:     price.Currency,
			Email:        email,
			PaymentToken: data.PaymentToken,
			OrderID:      subscriptionID.String(),
			PONumber:     subscriptionID.String(),
			CustomerID:   user.ID,
		}

		resp, err := client.AddRecurringSubscription(params)
		if err != nil {
			return nil, err
		}

		emailCopy := email
		subscription := &models.Subscription{
			UserID:                  user.ID,
			PriceID:                 priceID,
			ID:                      subscriptionID,
			ProcessorSubscriptionID: resp.SubscriptionID,
			Status:                  models.StatusPending,
			Processor:               models.Processor(processor),
			ProcessorProvider: func() *string {
				if provider == "" {
					return nil
				}
				pp := provider
				return &pp
			}(),
			UserEmail: &emailCopy,
		}

		if err := s.Create(ctx, subscription); err != nil {
			return nil, fmt.Errorf("failed to create subscription: %w", err)
		}
		return resp, nil
	default:
		return nil, errors.New("invalid payment processor")
	}
}

// GetUserSubscription retrieves the current subscription for a user
func (s *SubscriptionService) GetUserSubscription(ctx context.Context, userID string) (*models.Subscription, error) {
	return s.GetByUserID(ctx, userID)
}

// CancelUserSubscription cancels a user's subscription
func (s *SubscriptionService) CancelUserSubscription(ctx context.Context, userID string, feedback string) error {
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

	// Entitlements are managed in lifecycle and user flows

	// Add notification
	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    userID,
		EventType: models.NotificationPremiumEnded,
		Data: map[string]any{
			"reason": string(PremiumEndReasonUserCancel),
		},
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

func NewSubscriptionService(
	db *db.DB,
	priceService *PriceService,
	productService *ProductService,
	notificationQueueService *NotificationQueueService,
	ccbillRESTClient *ccbill.RESTClient,
	nmiClients map[string]*nmi.NMIClient,
) *SubscriptionService {
	return &SubscriptionService{
		subscriptionRepo:         repo.NewSubscriptionRepo(db),
		PriceService:             priceService,
		ProductService:           productService,
		NotificationQueueService: notificationQueueService,
		CCBillRESTClient:         ccbillRESTClient,
		NMIClients:               nmiClients,
	}
}

func (s *SubscriptionService) Create(ctx context.Context, subscription *models.Subscription) error {
	return s.subscriptionRepo.Create(ctx, subscription)
}

func (s *SubscriptionService) GetByID(ctx context.Context, id uuid.UUID) (*models.Subscription, error) {
	return s.subscriptionRepo.GetByID(ctx, id)
}

func (s *SubscriptionService) GetByUserID(ctx context.Context, id string) (*models.Subscription, error) {
	return s.subscriptionRepo.GetLatestByUserID(ctx, id)
}

func (s *SubscriptionService) GetByUserIDAndPriceID(ctx context.Context, id string, priceID uuid.UUID) (*models.Subscription, error) {
	return s.subscriptionRepo.GetByUserIDAndPriceID(ctx, id, priceID)
}

func (s *SubscriptionService) Update(ctx context.Context, subscription *models.Subscription) error {
	return s.subscriptionRepo.Update(ctx, subscription)
}

func (s *SubscriptionService) GetSubscribers(ctx context.Context, params query.QueryOptions[GetSubscriptionsFilters]) ([]*models.Subscription, int64, error) {
	repoParams := query.QueryOptions[repo.SubscriptionFilters]{
		Filters: repo.SubscriptionFilters{
			UserID:    params.Filters.UserID,
			Status:    params.Filters.Status,
			PriceID:   params.Filters.PriceID,
			Processor: params.Filters.Processor,
		},
		Limit:  params.Limit,
		Offset: params.Offset,
	}

	return s.subscriptionRepo.GetSubscribers(ctx, repoParams)
}

func (s *SubscriptionService) GetPaginatedByUserID(ctx context.Context, userID string, page, pageSize int) ([]models.Subscription, int, error) {
	return s.subscriptionRepo.GetPaginatedByUserID(ctx, userID, page, pageSize)
}

// GetSubscriptionsWithDetailsForUser retrieves subscriptions with related price information for billing history
func (s *SubscriptionService) GetSubscriptionsWithDetailsForUser(ctx context.Context, userID string, page, pageSize int) ([]models.Subscription, int, error) {
	return s.subscriptionRepo.GetSubscriptionsWithDetailsForUser(ctx, userID, page, pageSize)
}

// GetActiveSubscriptionsByUserID retrieves only active subscriptions for a user
func (s *SubscriptionService) GetActiveSubscriptionsByUserID(ctx context.Context, userID string) ([]models.Subscription, error) {
	return s.subscriptionRepo.GetActiveSubscriptionsByUserID(ctx, userID)
}

// GetSubscriptionsByProcessorAndUserID retrieves subscriptions filtered by processor
func (s *SubscriptionService) GetSubscriptionsByProcessorAndUserID(ctx context.Context, userID string, processor models.Processor) ([]models.Subscription, error) {
	return s.subscriptionRepo.GetSubscriptionsByProcessorAndUserID(ctx, userID, processor)
}

// GetActiveSubscription retrieves the active subscription for a user
func (s *SubscriptionService) GetActiveSubscription(ctx context.Context, userID string) (*models.Subscription, error) {
	return s.subscriptionRepo.GetActiveSubscription(ctx, userID)
}

// GetByProcessorSubscriptionID finds a subscription by processor, provider, and processor_subscription_id
func (s *SubscriptionService) GetByProcessorSubscriptionID(ctx context.Context, processor, provider, processorSubscriptionID string) (*models.Subscription, error) {
	return s.subscriptionRepo.GetByProcessorSubscriptionID(ctx, processor, provider, processorSubscriptionID)
}

// GetActiveSubscriptionsByProcessor gets all active subscriptions for a processor
func (s *SubscriptionService) GetActiveSubscriptionsByProcessor(ctx context.Context, processor string) ([]*models.Subscription, error) {
	return s.subscriptionRepo.GetActiveSubscriptionsByProcessor(ctx, processor)
}

// Delete removes a subscription from the database permanently
func (s *SubscriptionService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.subscriptionRepo.Delete(ctx, id)
}
