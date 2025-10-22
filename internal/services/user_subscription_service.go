package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/integrations/nmi"
	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Sentinel errors for subscription operations
var (
	ErrSubscriptionNotFound     = errors.New("subscription not found")
	ErrSubscriptionNotActive    = errors.New("subscription is not active")
	ErrNotificationNotFound     = errors.New("notification not found")
	ErrNotificationAccessDenied = errors.New("notification does not belong to user")
)

// UserSubscriptionService handles user-facing subscription operations
type UserSubscriptionService struct {
	SubscriptionService      *SubscriptionService
	ProductService           *ProductService
	PriceService             *PriceService
	PaymentService           *PaymentService
	NotificationQueueService *NotificationQueueService
	EntitlementService       *EntitlementService
	NMIClients               map[string]*nmi.NMIClient
}

// UserSubscriptionResponse represents a user's subscription with enriched data
type UserSubscriptionResponse struct {
	*models.Subscription
	Product *models.Product `json:"product,omitempty"`
	Price   *models.Price   `json:"price,omitempty"`
}

// GetUserSubscription retrieves the current subscription for a user with enriched data
func (s *UserSubscriptionService) GetUserSubscription(ctx context.Context, userID string) (*UserSubscriptionResponse, error) {
	subscription, err := s.SubscriptionService.GetActiveSubscription(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	response := &UserSubscriptionResponse{
		Subscription: subscription,
	}

	// Enrich with price and product data if available
	if subscription.PriceID != uuid.Nil {
		price, err := s.PriceService.GetByID(ctx, subscription.PriceID)
		if err == nil {
			response.Price = price

			// Get product data
			product, err := s.ProductService.GetByID(ctx, price.ProductID)
			if err == nil {
				response.Product = product
			}
		}
	}

	return response, nil
}

// GetUserSubscriptionHistory retrieves subscription history for a user
func (s *UserSubscriptionService) GetUserSubscriptionHistory(ctx context.Context, userID string, queryOpts *query.QueryOptions[GetSubscriptionsFilters]) ([]*UserSubscriptionResponse, int64, error) {
	// Set user filter
	if queryOpts.Filters.UserID == "" {
		queryOpts.Filters.UserID = userID
	}

	subscriptions, total, err := s.SubscriptionService.GetSubscribers(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get subscription history: %w", err)
	}
	queryOpts.SetTotal(total)

	responses := make([]*UserSubscriptionResponse, len(subscriptions))
	for i, sub := range subscriptions {
		responses[i] = &UserSubscriptionResponse{
			Subscription: sub,
		}

		// Enrich with price and product data if available
		if sub.PriceID != uuid.Nil {
			if price, err := s.PriceService.GetByID(ctx, sub.PriceID); err == nil {
				responses[i].Price = price

				if product, err := s.ProductService.GetByID(ctx, price.ProductID); err == nil {
					responses[i].Product = product
				}
			}
		}
	}

	return responses, total, nil
}

// GetUserPayments retrieves one-off purchases for a user
func (s *UserSubscriptionService) GetUserPayments(ctx context.Context, userID string, queryOpts *query.QueryOptions[GetPaymentsFilters]) ([]*models.Payment, int64, error) {
	// Set user filter
	if queryOpts.Filters.UserID == "" {
		queryOpts.Filters.UserID = userID
	}

	purchases, total, err := s.PaymentService.GetPayments(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get purchases: %w", err)
	}
	queryOpts.SetTotal(total)

	return purchases, total, nil
}

// GetUserNotifications retrieves notifications for a user
func (s *UserSubscriptionService) GetUserNotifications(ctx context.Context, userID string, queryOpts *query.QueryOptions[GetNotificationsFilters]) ([]*models.NotificationQueue, int64, error) {
	// Set user filter
	if queryOpts.Filters.UserID == "" {
		queryOpts.Filters.UserID = userID
	}

	notifications, total, err := s.NotificationQueueService.GetNotifications(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get notifications: %w", err)
	}
	queryOpts.SetTotal(total)

	return notifications, total, nil
}

// MarkNotificationRead marks a notification as read
func (s *UserSubscriptionService) MarkNotificationRead(ctx context.Context, userID string, notificationID uuid.UUID) error {
	notification, err := s.NotificationQueueService.GetByID(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotificationNotFound, err)
	}

	// Verify the notification belongs to the user
	if uid, err := uuid.Parse(userID); err != nil || notification.UserID != uid {
		return ErrNotificationAccessDenied
	}

	notification.MarkAsSeen() // Mark as seen (new boolean field)
	return s.NotificationQueueService.Update(ctx, notification)
}

// CancelUserSubscription cancels a user's subscription
func (s *UserSubscriptionService) CancelUserSubscription(ctx context.Context, userID string, feedback string) error {
	subscription, err := s.SubscriptionService.GetActiveSubscription(ctx, userID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSubscriptionNotFound, err)
	}

	if subscription.Processor != models.ProcessorNMI {
		return fmt.Errorf("unable to cancel subscription for processor %s", subscription.Processor)
	}

	// End entitlements for this subscription now
	// if s.EntitlementService != nil {
	// 	reason := models.EntitlementRevokeAdmin
	// 	if err := s.EntitlementService.EndActiveBySubscription(ctx, subscription.ID, now, &reason); err != nil {
	// 		log.WithFields(log.Fields{
	// 			"subscription_id": subscription.ID,
	// 			"user_id":         userID,
	// 			"error":           err.Error(),
	// 		}).Error("Failed to end entitlements during subscription cancellation")
	// 	}
	// }

	// Cancel subscription with NMI
	if s.NMIClients != nil {
		provider := ""
		if subscription.ProcessorProvider != nil {
			provider = strings.ToLower(strings.TrimSpace(*subscription.ProcessorProvider))
		}
		if provider == "" && subscription.Price != nil && subscription.Price.NMIProvider != nil {
			provider = strings.ToLower(strings.TrimSpace(*subscription.Price.NMIProvider))
		}
		if provider == "" {
			provider = "mobius"
		}

		if client, ok := s.NMIClients[provider]; ok && subscription.Price != nil && subscription.Price.NMIPlanID != nil {
			if err := client.DeleteRecurringSubscription(*subscription.Price.NMIPlanID); err != nil {
				return fmt.Errorf("failed to cancel subscription with NMI provider '%s': %w", provider, err)
			}
		}
	}

	// Add notification
	uid, perr := uuid.Parse(userID)
	if perr != nil {
		return fmt.Errorf("invalid user id: %w", perr)
	}
	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    uid,
		EventType: models.NotificationPremiumEnded,
		Data:      map[string]any{"reason": string(PremiumEndReasonUserCancel)},
	}
	if err := s.NotificationQueueService.Create(ctx, notification); err != nil {
		log.WithFields(log.Fields{
			"subscription_id":   subscription.ID,
			"user_id":           userID,
			"notification_type": notification.EventType,
			"error":             err.Error(),
		}).Error("Failed to create notification during subscription cancellation")
	}

	return nil
}

// NewUserSubscriptionService creates a new UserSubscriptionService
func NewUserSubscriptionService(
	subscriptionService *SubscriptionService,
	productService *ProductService,
	priceService *PriceService,
	paymentService *PaymentService,
	notificationQueueService *NotificationQueueService,
	entitlementService *EntitlementService,
	nmiClients map[string]*nmi.NMIClient,
) *UserSubscriptionService {
	return &UserSubscriptionService{
		NMIClients:               nmiClients,
		SubscriptionService:      subscriptionService,
		ProductService:           productService,
		PriceService:             priceService,
		PaymentService:           paymentService,
		NotificationQueueService: notificationQueueService,
		EntitlementService:       entitlementService,
	}
}
