package services

import (
	"errors"

	"github.com/doujins-org/doujins-billing/internal/db"
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
	DB *db.DB
}

// // GetSubscriptionByUserID retrieves a subscription by user ID
// func (s *UserSubscriptionService) GetSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (*models.Subscription, error) {
// 	var subscription models.Subscription
// 	err := s.DB.GetDB().NewSelect().
// 		Model(&subscription).
// 		Where("user_id = ?", userID).
// 		Where("status = ?", models.StatusActive).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get subscription by user ID: %w", err)
// 	}
// 	return &subscription, nil
// }

// // GetPriceByID retrieves a price by its ID
// func (s *UserSubscriptionService) GetPriceByID(ctx context.Context, id uuid.UUID) (*models.Price, error) {
// 	var price models.Price
// 	err := s.DB.GetDB().NewSelect().
// 		Model(&price).
// 		Where("id = ?", id).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get price by ID: %w", err)
// 	}
// 	return &price, nil
// }

// // GetProductByID retrieves a product by its ID
// func (s *UserSubscriptionService) GetProductByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
// 	var product models.Product
// 	err := s.DB.GetDB().NewSelect().
// 		Model(&product).
// 		Where("id = ?", id).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get product by ID: %w", err)
// 	}
// 	return &product, nil
// }

// // GetSubscribers retrieves subscribers with pagination
// func (s *UserSubscriptionService) GetSubscribers(ctx context.Context, opts query.QueryOptions) ([]*models.Subscription, int64, error) {
// 	var subscriptions []*models.Subscription
// 	var total int64

// 	// Count total records
// 	count, err := s.DB.GetDB().NewSelect().
// 		Model((*models.Subscription)(nil)).
// 		Count(ctx)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to count subscribers: %w", err)
// 	}
// 	total = int64(count)

// 	// Get paginated results
// 	err = s.DB.GetDB().NewSelect().
// 		Model(&subscriptions).
// 		Limit(opts.Limit).
// 		Offset(opts.Offset).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to get subscribers: %w", err)
// 	}

// 	return subscriptions, total, nil
// }

// // GetPurchases retrieves purchases with pagination
// func (s *UserSubscriptionService) GetPurchases(ctx context.Context, opts query.QueryOptions) ([]*models.Purchase, int64, error) {
// 	var purchases []*models.Purchase
// 	var total int64

// 	// Count total records
// 	count, err := s.DB.GetDB().NewSelect().
// 		Model((*models.Purchase)(nil)).
// 		Count(ctx)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to count purchases: %w", err)
// 	}
// 	total = int64(count)

// 	// Get paginated results
// 	err = s.DB.GetDB().NewSelect().
// 		Model(&purchases).
// 		Limit(opts.Limit).
// 		Offset(opts.Offset).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to get purchases: %w", err)
// 	}

// 	return purchases, total, nil
// }

// // GetNotifications retrieves notifications with pagination
// func (s *UserSubscriptionService) GetNotifications(ctx context.Context, opts query.QueryOptions) ([]*models.NotificationQueue, int64, error) {
// 	var notifications []*models.NotificationQueue
// 	var total int64

// 	// Count total records
// 	count, err := s.DB.GetDB().NewSelect().
// 		Model((*models.NotificationQueue)(nil)).
// 		Count(ctx)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to count notifications: %w", err)
// 	}
// 	total = int64(count)

// 	// Get paginated results
// 	err = s.DB.GetDB().NewSelect().
// 		Model(&notifications).
// 		Limit(opts.Limit).
// 		Offset(opts.Offset).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to get notifications: %w", err)
// 	}

// 	return notifications, total, nil
// }

// // GetNotificationByID retrieves a notification by its ID
// func (s *UserSubscriptionService) GetNotificationByID(ctx context.Context, id uuid.UUID) (*models.NotificationQueue, error) {
// 	var notification models.NotificationQueue
// 	err := s.DB.GetDB().NewSelect().
// 		Model(&notification).
// 		Where("id = ?", id).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get notification by ID: %w", err)
// 	}
// 	return &notification, nil
// }

// // UpdateNotification updates a notification
// func (s *UserSubscriptionService) UpdateNotification(ctx context.Context, notification *models.NotificationQueue) error {
// 	_, err := s.DB.GetDB().NewUpdate().
// 		Model(notification).
// 		Where("id = ?", notification.ID).
// 		Exec(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to update notification: %w", err)
// 	}
// 	return nil
// }

// // UpdateSubscription updates a subscription
// func (s *UserSubscriptionService) UpdateSubscription(ctx context.Context, subscription *models.Subscription) error {
// 	_, err := s.DB.GetDB().NewUpdate().
// 		Model(subscription).
// 		Where("id = ?", subscription.ID).
// 		Exec(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to update subscription: %w", err)
// 	}
// 	return nil
// }

// // RevokeUserRolesBySubSourceID revokes user roles by subscription source ID
// func (s *UserSubscriptionService) RevokeUserRolesBySubSourceID(ctx context.Context, subID uuid.UUID) error {
// 	_, err := s.DB.GetDB().NewUpdate().
// 		Model((*models.UserRoleGrant)(nil)).
// 		Set("revoked_at = ?", time.Now()).
// 		Where("sub_source_id = ?", subID).
// 		Where("revoked_at IS NULL").
// 		Exec(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to revoke user roles: %w", err)
// 	}
// 	return nil
// }

// // CreateNotification creates a new notification
// func (s *UserSubscriptionService) CreateNotification(ctx context.Context, notification *models.NotificationQueue) error {
// 	_, err := s.DB.GetDB().NewInsert().
// 		Model(notification).
// 		Exec(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to create notification: %w", err)
// 	}
// 	return nil
// }

// // UserSubscriptionResponse represents a user's subscription with enriched data
// type UserSubscriptionResponse struct {
// 	*models.Subscription
// 	Product *models.Product `json:"product,omitempty"`
// 	Price   *models.Price   `json:"price,omitempty"`
// }

// // GetUserSubscription retrieves the current subscription for a user with enriched data
// func (s *UserSubscriptionService) GetUserSubscription(ctx context.Context, userID uuid.UUID) (*UserSubscriptionResponse, error) {
// 	subscription, err := s.GetSubscriptionByUserID(ctx, userID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get subscription: %w", err)
// 	}

// 	response := &UserSubscriptionResponse{
// 		Subscription: subscription,
// 	}

// 	// Enrich with price and product data if available
// 	if subscription.PriceID != uuid.Nil {
// 		price, err := s.GetPriceByID(ctx, subscription.PriceID)
// 		if err == nil {
// 			response.Price = price

// 			// Get product data
// 			product, err := s.GetProductByID(ctx, price.ProductID)
// 			if err == nil {
// 				response.Product = product
// 			}
// 		}
// 	}

// 	return response, nil
// }

// // GetUserSubscriptionHistory retrieves subscription history for a user
// func (s *UserSubscriptionService) GetUserSubscriptionHistory(ctx context.Context, userID uuid.UUID, queryOpts *query.QueryOptions[repo.GetSubscriptionsFilters]) ([]*UserSubscriptionResponse, int64, error) {
// 	// Set user filter
// 	if queryOpts.Filters.UserID == uuid.Nil {
// 		queryOpts.Filters.UserID = userID
// 	}

// 	subscriptions, total, err := s.GetSubscribers(ctx, *queryOpts)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to get subscription history: %w", err)
// 	}

// 	responses := make([]*UserSubscriptionResponse, len(subscriptions))
// 	for i, sub := range subscriptions {
// 		responses[i] = &UserSubscriptionResponse{
// 			Subscription: sub,
// 		}

// 		// Enrich with price and product data if available
// 		if sub.PriceID != uuid.Nil {
// 			if price, err := s.GetPriceByID(ctx, sub.PriceID); err == nil {
// 				responses[i].Price = price

// 				if product, err := s.GetProductByID(ctx, price.ProductID); err == nil {
// 					responses[i].Product = product
// 				}
// 			}
// 		}
// 	}

// 	return responses, total, nil
// }

// // GetUserPurchases retrieves one-off purchases for a user
// func (s *UserSubscriptionService) GetUserPurchases(ctx context.Context, userID uuid.UUID, queryOpts *query.QueryOptions[repo.GetPurchasesFilters]) ([]*models.Purchase, int64, error) {
// 	// Set user filter
// 	if queryOpts.Filters.UserID == uuid.Nil {
// 		queryOpts.Filters.UserID = userID
// 	}

// 	purchases, total, err := s.GetPurchases(ctx, *queryOpts)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to get purchases: %w", err)
// 	}

// 	return purchases, total, nil
// }

// // GetUserNotifications retrieves notifications for a user
// func (s *UserSubscriptionService) GetUserNotifications(ctx context.Context, userID uuid.UUID, queryOpts *query.QueryOptions[repo.GetNotificationsFilters]) ([]*models.NotificationQueue, int64, error) {
// 	// Set user filter
// 	if queryOpts.Filters.UserID == uuid.Nil {
// 		queryOpts.Filters.UserID = userID
// 	}

// 	notifications, total, err := s.GetNotifications(ctx, *queryOpts)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to get notifications: %w", err)
// 	}

// 	return notifications, total, nil
// }

// // MarkNotificationRead marks a notification as read
// func (s *UserSubscriptionService) MarkNotificationRead(ctx context.Context, userID, notificationID uuid.UUID) error {
// 	notification, err := s.GetNotificationByID(ctx, notificationID)
// 	if err != nil {
// 		return fmt.Errorf("%w: %w", ErrNotificationNotFound, err)
// 	}

// 	// Verify the notification belongs to the user
// 	if notification.UserID != userID {
// 		return ErrNotificationAccessDenied
// 	}

// 	notification.MarkAsSeen() // Mark as seen (new boolean field)
// 	return s.UpdateNotification(ctx, notification)
// }

// // CancelUserSubscription cancels a user's subscription
// func (s *UserSubscriptionService) CancelUserSubscription(ctx context.Context, userID uuid.UUID, feedback string) error {
// 	subscription, err := s.GetSubscriptionByUserID(ctx, userID)
// 	if err != nil {
// 		return fmt.Errorf("%w: %w", ErrSubscriptionNotFound, err)
// 	}

// 	if subscription.Status != models.StatusActive {
// 		return ErrSubscriptionNotActive
// 	}

// 	now := time.Now()
// 	cancelType := models.CancelTypeUser
// 	subscription.Status = models.StatusCancelled
// 	subscription.CancelledAt = &now
// 	subscription.CancelType = &cancelType
// 	if feedback != "" {
// 		subscription.CancelFeedback = &feedback
// 	}

// 	if err := s.UpdateSubscription(ctx, subscription); err != nil {
// 		return fmt.Errorf("failed to update subscription: %w", err)
// 	}

// 	// Revoke role grants
// 	if err := s.RevokeUserRolesBySubSourceID(ctx, subscription.ID); err != nil {
// 		log.WithFields(log.Fields{
// 			"subscription_id": subscription.ID,
// 			"user_id":         userID,
// 			"error":           err.Error(),
// 		}).Error("Failed to revoke role grants during subscription cancellation")
// 	}

// 	// Add notification
// 	notification := &models.NotificationQueue{
// 		ID:        uuid.New(),
// 		UserID:    userID,
// 		EventType: models.NotificationPremiumEnded,
// 	}
// 	if err := s.CreateNotification(ctx, notification); err != nil {
// 		log.WithFields(log.Fields{
// 			"subscription_id":   subscription.ID,
// 			"user_id":           userID,
// 			"notification_type": notification.EventType,
// 			"error":             err.Error(),
// 		}).Error("Failed to create notification during subscription cancellation")
// 	}

// 	return nil
// }
