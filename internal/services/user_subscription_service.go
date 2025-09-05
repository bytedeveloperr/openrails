package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db/models"
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
	UserRoleGrantService     *UserRoleGrantService
}

// UserSubscriptionResponse represents a user's subscription with enriched data
type UserSubscriptionResponse struct {
	*models.Subscription
	Product *models.Product `json:"product,omitempty"`
	Price   *models.Price   `json:"price,omitempty"`
}

// GetUserSubscription retrieves the current subscription for a user with enriched data
func (s *UserSubscriptionService) GetUserSubscription(ctx context.Context, userID string) (*UserSubscriptionResponse, error) {
	subscription, err := s.SubscriptionService.GetByUserID(ctx, userID)
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

// GetUserPurchases retrieves one-off purchases for a user
func (s *UserSubscriptionService) GetUserPurchases(ctx context.Context, userID string, queryOpts *query.QueryOptions[GetPaymentsFilters]) ([]*models.Payment, int64, error) {
	// Set user filter
    if queryOpts.Filters.UserID == "" {
        queryOpts.Filters.UserID = userID
    }

	purchases, total, err := s.PaymentService.GetPayments(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get purchases: %w", err)
	}

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

	return notifications, total, nil
}

// MarkNotificationRead marks a notification as read
func (s *UserSubscriptionService) MarkNotificationRead(ctx context.Context, userID string, notificationID uuid.UUID) error {
	notification, err := s.NotificationQueueService.GetByID(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotificationNotFound, err)
	}

	// Verify the notification belongs to the user
    if notification.UserID != userID {
        return ErrNotificationAccessDenied
    }

	notification.MarkAsSeen() // Mark as seen (new boolean field)
	return s.NotificationQueueService.Update(ctx, notification)
}

// CancelUserSubscription cancels a user's subscription
func (s *UserSubscriptionService) CancelUserSubscription(ctx context.Context, userID string, feedback string) error {
	subscription, err := s.SubscriptionService.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSubscriptionNotFound, err)
	}

	if subscription.Status != models.StatusActive {
		return ErrSubscriptionNotActive
	}

	now := time.Now()
	cancelType := models.CancelTypeUser
	subscription.Status = models.StatusCancelled
	subscription.CancelledAt = &now
	subscription.CancelType = &cancelType
	if feedback != "" {
		subscription.CancelFeedback = &feedback
	}

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Revoke role grants
	if err := s.UserRoleGrantService.RevokeBySubSourceID(ctx, subscription.ID); err != nil {
		log.WithFields(log.Fields{
			"subscription_id": subscription.ID,
			"user_id":         userID,
			"error":           err.Error(),
		}).Error("Failed to revoke role grants during subscription cancellation")
	}

	// Add notification
	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    userID,
		EventType: models.NotificationPremiumEnded,
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
