package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/pkg/query"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/supabase-community/gotrue-go/types"
)

// Additional sentinel errors for admin operations
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleGrantNotFound = errors.New("role grant not found")
)

// AdminSubscriptionService handles administrative subscription operations
type AdminSubscriptionService struct {
	DB *db.DB
}

// GetSubscribers retrieves subscribers with pagination and admin details
func (s *AdminSubscriptionService) GetSubscribers(ctx context.Context, opts query.QueryOptions) ([]*models.Subscription, int64, error) {
	var subscriptions []*models.Subscription
	var total int64

	// Count total records
	count, err := s.DB.GetDB().NewSelect().
		Model((*models.Subscription)(nil)).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count subscribers: %w", err)
	}
	total = int64(count)

	// Get paginated results
	err = s.DB.GetDB().NewSelect().
		Model(&subscriptions).
		Limit(opts.Limit).
		Offset(opts.Offset).
		Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get subscribers: %w", err)
	}

	return subscriptions, total, nil
}

// GetSubscriptionByID retrieves a subscription by its ID
func (s *AdminSubscriptionService) GetSubscriptionByID(ctx context.Context, id uuid.UUID) (*models.Subscription, error) {
	var subscription models.Subscription
	err := s.DB.GetDB().NewSelect().
		Model(&subscription).
		Where("id = ?", id).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription by ID: %w", err)
	}
	return &subscription, nil
}

// GetSubscriptionByUserID retrieves a subscription by user ID
func (s *AdminSubscriptionService) GetSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (*models.Subscription, error) {
	var subscription models.Subscription
	err := s.DB.GetDB().NewSelect().
		Model(&subscription).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription by user ID: %w", err)
	}
	return &subscription, nil
}

// UpdateSubscription updates a subscription
func (s *AdminSubscriptionService) UpdateSubscription(ctx context.Context, subscription *models.Subscription) error {
	_, err := s.DB.GetDB().NewUpdate().
		Model(subscription).
		Where("id = ?", subscription.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}
	return nil
}

// GetPriceByID retrieves a price by its ID
func (s *AdminSubscriptionService) GetPriceByID(ctx context.Context, id uuid.UUID) (*models.Price, error) {
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

// GetProductByID retrieves a product by its ID
func (s *AdminSubscriptionService) GetProductByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	var product models.Product
	err := s.DB.GetDB().NewSelect().
		Model(&product).
		Where("id = ?", id).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get product by ID: %w", err)
	}
	return &product, nil
}

// GetGoTrueUserByID retrieves a user by ID from GoTrue
func (s *AdminSubscriptionService) GetGoTrueUserByID(ctx context.Context, userID uuid.UUID) (*types.User, error) {
	// This would typically call the GoTrue API or database
	// For now, return a placeholder implementation
	return nil, fmt.Errorf("user not found: %s", userID)
}

// CreateProduct creates a new product
func (s *AdminSubscriptionService) CreateProduct(ctx context.Context, product *models.Product) error {
	_, err := s.DB.GetDB().NewInsert().
		Model(product).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create product: %w", err)
	}
	return nil
}

// UpdateProduct updates a product
func (s *AdminSubscriptionService) UpdateProduct(ctx context.Context, product *models.Product) error {
	_, err := s.DB.GetDB().NewUpdate().
		Model(product).
		Where("id = ?", product.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update product: %w", err)
	}
	return nil
}

// CreatePrice creates a new price
func (s *AdminSubscriptionService) CreatePrice(ctx context.Context, price *models.Price) error {
	_, err := s.DB.GetDB().NewInsert().
		Model(price).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create price: %w", err)
	}
	return nil
}

// GetUserRoleGrants retrieves user role grants with pagination
func (s *AdminSubscriptionService) GetUserRoleGrants(ctx context.Context, opts query.QueryOptions) ([]*models.UserRoleGrant, int64, error) {
	var grants []*models.UserRoleGrant
	var total int64

	// Count total records
	count, err := s.DB.GetDB().NewSelect().
		Model((*models.UserRoleGrant)(nil)).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count user role grants: %w", err)
	}
	total = int64(count)

	// Get paginated results
	err = s.DB.GetDB().NewSelect().
		Model(&grants).
		Limit(opts.Limit).
		Offset(opts.Offset).
		Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get user role grants: %w", err)
	}

	return grants, total, nil
}

// CreatePermanentGrant creates a permanent role grant
func (s *AdminSubscriptionService) CreatePermanentGrant(ctx context.Context, userID, roleID uuid.UUID) (*models.UserRoleGrant, error) {
	grant := &models.UserRoleGrant{
		ID:        uuid.New(),
		UserID:    userID,
		RoleID:    roleID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		// ExpiresAt is nil for permanent grants
	}

	_, err := s.DB.GetDB().NewInsert().
		Model(grant).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create permanent grant: %w", err)
	}
	return grant, nil
}

// ExtendRoleExpiration extends role expiration for a user
func (s *AdminSubscriptionService) ExtendRoleExpiration(ctx context.Context, userID, roleID uuid.UUID, days int) (*models.UserRoleGrant, time.Time, error) {
	// Check if user already has this role
	var existingGrant models.UserRoleGrant
	err := s.DB.GetDB().NewSelect().
		Model(&existingGrant).
		Where("user_id = ?", userID).
		Where("role_id = ?", roleID).
		Where("revoked_at IS NULL").
		Scan(ctx)

	var newExpirationDate time.Time
	if err == nil {
		// User has existing grant, extend it
		if existingGrant.ExpiresAt != nil {
			newExpirationDate = existingGrant.ExpiresAt.AddDate(0, 0, days)
		} else {
			newExpirationDate = time.Now().AddDate(0, 0, days)
		}
		existingGrant.ExpiresAt = &newExpirationDate

		_, updateErr := s.DB.GetDB().NewUpdate().
			Model(&existingGrant).
			Where("id = ?", existingGrant.ID).
			Exec(ctx)
		if updateErr != nil {
			return nil, time.Time{}, fmt.Errorf("failed to update role expiration: %w", updateErr)
		}
		return &existingGrant, newExpirationDate, nil
	} else {
		// Create new role grant
		newExpirationDate = time.Now().AddDate(0, 0, days)
		newGrant := &models.UserRoleGrant{
			ID:        uuid.New(),
			UserID:    userID,
			RoleID:    roleID,
			ExpiresAt: &newExpirationDate,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		_, insertErr := s.DB.GetDB().NewInsert().
			Model(newGrant).
			Exec(ctx)
		if insertErr != nil {
			return nil, time.Time{}, fmt.Errorf("failed to create role grant: %w", insertErr)
		}
		return newGrant, newExpirationDate, nil
	}
}

// CreatePurchase creates a new purchase
func (s *AdminSubscriptionService) CreatePurchase(ctx context.Context, purchase *models.Purchase) error {
	_, err := s.DB.GetDB().NewInsert().
		Model(purchase).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create purchase: %w", err)
	}
	return nil
}

// UpdatePurchase updates a purchase
func (s *AdminSubscriptionService) UpdatePurchase(ctx context.Context, purchase *models.Purchase) error {
	_, err := s.DB.GetDB().NewUpdate().
		Model(purchase).
		Where("id = ?", purchase.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update purchase: %w", err)
	}
	return nil
}

// DeleteUserRoleGrant deletes a user role grant
func (s *AdminSubscriptionService) DeleteUserRoleGrant(ctx context.Context, grantID uuid.UUID) error {
	_, err := s.DB.GetDB().NewDelete().
		Model((*models.UserRoleGrant)(nil)).
		Where("id = ?", grantID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user role grant: %w", err)
	}
	return nil
}

// GetPurchases retrieves purchases with pagination
func (s *AdminSubscriptionService) GetPurchases(ctx context.Context, opts query.QueryOptions) ([]*models.Purchase, int64, error) {
	var purchases []*models.Purchase
	var total int64

	// Count total records
	count, err := s.DB.GetDB().NewSelect().
		Model((*models.Purchase)(nil)).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count purchases: %w", err)
	}
	total = int64(count)

	// Get paginated results
	err = s.DB.GetDB().NewSelect().
		Model(&purchases).
		Limit(opts.Limit).
		Offset(opts.Offset).
		Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get purchases: %w", err)
	}

	return purchases, total, nil
}

// GetNotifications retrieves notifications with pagination
func (s *AdminSubscriptionService) GetNotifications(ctx context.Context, opts query.QueryOptions) ([]*models.NotificationQueue, int64, error) {
	var notifications []*models.NotificationQueue
	var total int64

	// Count total records
	count, err := s.DB.GetDB().NewSelect().
		Model((*models.NotificationQueue)(nil)).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count notifications: %w", err)
	}
	total = int64(count)

	// Get paginated results
	err = s.DB.GetDB().NewSelect().
		Model(&notifications).
		Limit(opts.Limit).
		Offset(opts.Offset).
		Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get notifications: %w", err)
	}

	return notifications, total, nil
}

// CreateNotification creates a new notification
func (s *AdminSubscriptionService) CreateNotification(ctx context.Context, notification *models.NotificationQueue) error {
	_, err := s.DB.GetDB().NewInsert().
		Model(notification).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}
	return nil
}

// RevokeUserRolesBySubSourceID revokes user roles by subscription source ID
func (s *AdminSubscriptionService) RevokeUserRolesBySubSourceID(ctx context.Context, subID uuid.UUID) error {
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

// AdminSubscriptionResponse represents a subscription with enriched admin data
type AdminSubscriptionResponse struct {
	*models.Subscription
	User    *types.User     `json:"user,omitempty"`
	Product *models.Product `json:"product,omitempty"`
	Price   *models.Price   `json:"price,omitempty"`
}

// GetAllSubscriptions retrieves all subscriptions with filtering (admin)
func (s *AdminSubscriptionService) GetAllSubscriptions(ctx context.Context, queryOpts *query.QueryOptions[repo.GetSubscriptionsFilters]) ([]*AdminSubscriptionResponse, int64, error) {
	subscriptions, total, err := s.SubscriptionRepo.GetSubscribers(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get subscriptions: %w", err)
	}

	responses := make([]*AdminSubscriptionResponse, len(subscriptions))
	for i, sub := range subscriptions {
		responses[i] = &AdminSubscriptionResponse{
			Subscription: sub,
		}

		// Enrich with user data
		if user, err := servic.GetGoTrueUserByID(ctx, sub.UserID); err == nil {
			responses[i].User = user
		}

		// Enrich with price and product data if available
		if price, err := s.PriceRepo.GetByID(ctx, sub.PriceID); err == nil {
			responses[i].Price = price

			if product, err := s.ProductRepo.GetByID(ctx, price.ProductID); err == nil {
				responses[i].Product = product
			}
		}
	}

	return responses, total, nil
}

// GetSubscriptionByID retrieves a specific subscription with full details (admin)
func (s *AdminSubscriptionService) GetSubscriptionByID(ctx context.Context, subscriptionID uuid.UUID) (*AdminSubscriptionResponse, error) {
	subscription, err := s.SubscriptionRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("subscription not found: %w", err)
	}

	response := &AdminSubscriptionResponse{
		Subscription: subscription,
	}

	// Enrich with price and product data if available
	if price, err := s.PriceRepo.GetByID(ctx, subscription.PriceID); err == nil {
		response.Price = price

		if product, err := s.ProductRepo.GetByID(ctx, price.ProductID); err == nil {
			response.Product = product
		}
	}

	// Enrich with user data
	if user, err := servic.GetGoTrueUserByID(ctx, subscription.UserID); err == nil {
		response.User = user
	}

	return response, nil
}

// UpdateSubscription updates a subscription (admin)
func (s *AdminSubscriptionService) UpdateSubscription(ctx context.Context, subscriptionID uuid.UUID, updates map[string]any) error {
	subscription, err := s.SubscriptionRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	// Apply allowed updates
	for field, value := range updates {
		switch field {
		case "status":
			if status, ok := value.(models.SubscriptionStatus); ok {
				subscription.Status = status
			}
		case "notes":
			if notes, ok := value.(string); ok {
				// Store notes in Metadata JSONB field which is designed for additional metadata
				var responseData map[string]any
				if subscription.Metadata != nil {
					if err := json.Unmarshal(subscription.Metadata, &responseData); err != nil {
						responseData = make(map[string]any)
					}
				} else {
					responseData = make(map[string]any)
				}
				responseData["admin_notes"] = notes
				if newData, err := json.Marshal(responseData); err == nil {
					subscription.Metadata = newData
				}
			}
		}
	}

	if err := s.UpdateSubscription(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	return nil
}

// CancelSubscription cancels a subscription (admin)
func (s *AdminSubscriptionService) CancelSubscription(ctx context.Context, subscriptionID uuid.UUID, reason string) error {
	subscription, err := s.SubscriptionRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	if subscription.Status != models.StatusActive {
		return fmt.Errorf("subscription is not active")
	}

	now := time.Now()
	cancelType := models.CancelTypeMerchant
	subscription.Status = models.StatusCancelled
	subscription.CancelledAt = &now
	subscription.CancelType = &cancelType
	if reason != "" {
		subscription.CancelFeedback = &reason
	}

	if err := s.UpdateSubscription(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Revoke role grants
	if err := s.UserRoleGrantRepo.RevokeBySubSourceID(ctx, subscription.ID); err != nil {
		log.WithFields(log.Fields{
			"subscription_id": subscription.ID,
			"user_id":         subscription.UserID,
			"error":           err.Error(),
		}).Error("Failed to revoke role grants during admin subscription operation")
	}

	// Add notification
	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    subscription.UserID,
		EventType: models.NotificationPremiumEnded,
	}
	if err := s.CreateNotification(ctx, notification); err != nil {
		log.WithFields(log.Fields{
			"subscription_id":   subscription.ID,
			"user_id":           subscription.UserID,
			"notification_type": notification.EventType,
			"error":             err.Error(),
		}).Error("Failed to create notification during admin subscription operation")
	}

	return nil
}

// CancelUserSubscription cancels a user's subscription by user ID (admin)
func (s *AdminSubscriptionService) CancelUserSubscription(ctx context.Context, userID uuid.UUID, reason string) error {
	subscription, err := s.SubscriptionRepo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSubscriptionNotFound, err)
	}

	if subscription.Status != models.StatusActive {
		return ErrSubscriptionNotActive
	}

	now := time.Now()
	cancelType := models.CancelTypeMerchant // Admin cancellation
	subscription.Status = models.StatusCancelled
	subscription.CancelledAt = &now
	subscription.CancelType = &cancelType
	if reason != "" {
		subscription.CancelFeedback = &reason
	}

	if err := s.UpdateSubscription(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Revoke role grants
	if err := s.UserRoleGrantRepo.RevokeBySubSourceID(ctx, subscription.ID); err != nil {
		log.WithFields(log.Fields{
			"subscription_id": subscription.ID,
			"user_id":         subscription.UserID,
			"error":           err.Error(),
		}).Error("Failed to revoke role grants during admin subscription operation")
	}

	// Add notification
	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    subscription.UserID,
		EventType: models.NotificationPremiumEnded,
	}
	if err := s.CreateNotification(ctx, notification); err != nil {
		log.WithFields(log.Fields{
			"subscription_id":   subscription.ID,
			"user_id":           subscription.UserID,
			"notification_type": notification.EventType,
			"error":             err.Error(),
		}).Error("Failed to create notification during admin subscription operation")
	}

	return nil
}

// ExtendSubscription extends a subscription period (admin)
func (s *AdminSubscriptionService) ExtendSubscription(ctx context.Context, subscriptionID uuid.UUID, days int, reason string) error {
	subscription, err := s.SubscriptionRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	if subscription.Status != models.StatusActive {
		return fmt.Errorf("subscription is not active")
	}

	// Extend the subscription period
	extension := time.Duration(days) * 24 * time.Hour

	if subscription.CurrentPeriodEndsAt != nil {
		newEndTime := subscription.CurrentPeriodEndsAt.Add(extension)
		subscription.CurrentPeriodEndsAt = &newEndTime
	} else {
		newEndTime := time.Now().Add(extension)
		subscription.CurrentPeriodEndsAt = &newEndTime
		startTime := time.Now()
		subscription.CurrentPeriodStartsAt = &startTime
	}

	if err := s.UpdateSubscription(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Admin actions don't generate user notifications - this is purely for admin operations

	return nil
}

// CreateProduct creates a new product (admin)
func (s *AdminSubscriptionService) CreateProduct(ctx context.Context, product *models.Product) error {
	product.ID = uuid.New()
	return s.ProductRepo.Create(ctx, product)
}

// UpdateProduct updates a product (admin)
func (s *AdminSubscriptionService) UpdateProduct(ctx context.Context, productID uuid.UUID, updates map[string]any) error {
	product, err := s.ProductRepo.GetByID(ctx, productID)
	if err != nil {
		return fmt.Errorf("product not found: %w", err)
	}

	// Apply allowed updates
	for field, value := range updates {
		switch field {
		case "name":
			if name, ok := value.(string); ok {
				product.DisplayName = name
			}
		case "description":
			if desc, ok := value.(string); ok {
				product.Description = desc
			}
		case "is_active":
			if active, ok := value.(bool); ok {
				product.IsActive = active
			}
		}
	}

	return s.ProductRepo.Update(ctx, product)
}

// CreatePrice creates a new price (admin)
func (s *AdminSubscriptionService) CreatePrice(ctx context.Context, price *models.Price) error {
	price.ID = uuid.New()
	return s.PriceRepo.Create(ctx, price)
}

// GetAllUserRoleGrants retrieves all user role grants with filtering (admin)
func (s *AdminSubscriptionService) GetAllUserRoleGrants(ctx context.Context, queryOpts *query.QueryOptions[repo.GetUserRoleGrantsFilters]) ([]*models.UserRoleGrant, int64, error) {
	grants, total, err := s.UserRoleGrantRepo.GetUserRoleGrants(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get role grants: %w", err)
	}

	return grants, total, nil
}

// CreateManualRoleGrant creates a manual role grant (admin)
func (s *AdminSubscriptionService) CreateManualRoleGrant(ctx context.Context, userID, roleID uuid.UUID, durationDays *int) error {
	if durationDays == nil {
		// Permanent manual grant - create with no expiration
		_, err := s.UserRoleGrantRepo.CreatePermanentGrant(ctx, userID, roleID)
		return err
	} else {
		// Temporary manual grant - extend existing or create new with expiration
		_, _, err := s.UserRoleGrantRepo.ExtendRoleExpiration(ctx, userID, roleID, *durationDays)
		return err
	}
}

// VerifyPayPalPurchase verifies a PayPal payment and grants the associated role (admin)
// This is used when admins manually verify PayPal payments outside the automated system
func (s *AdminSubscriptionService) VerifyPayPalPurchase(ctx context.Context, userID, priceID uuid.UUID, paypalTransactionID string) error {
	// Get price information
	price, err := s.PriceRepo.GetByID(ctx, priceID)
	if err != nil {
		return fmt.Errorf("failed to get price: %w", err)
	}

	// Get product information to determine role
	product, err := s.ProductRepo.GetByID(ctx, price.ProductID)
	if err != nil {
		return fmt.Errorf("failed to get product: %w", err)
	}

	// Create purchase record for audit trail
	purchase := &models.Purchase{
		ID:            uuid.New(),
		UserID:        userID,
		PriceID:       priceID,
		Processor:     models.ProcessorPayPal,
		TransactionID: paypalTransactionID,
		Amount:        price.Amount, // Use price.Amount instead of paidAmount parameter
		Currency:      price.Currency,
		PurchasedAt:   time.Now(),
	}

	if err := s.PurchaseRepo.Create(ctx, purchase); err != nil {
		return fmt.Errorf("failed to create purchase record: %w", err)
	}

	// Only grant role if product has one (handle nullable RoleID)
	if product.RoleID != nil {
		// Determine role duration from product or default
		var durationDays int
		if product.RoleDurationDays != nil && *product.RoleDurationDays > 0 {
			durationDays = *product.RoleDurationDays
		} else {
			// Default to 30 days if no duration specified
			durationDays = 30
		}

		// Extend the user's existing role expiration or create new grant
		grant, newExpirationDate, err := s.UserRoleGrantRepo.ExtendRoleExpiration(ctx, userID, *product.RoleID, durationDays)
		if err != nil {
			return fmt.Errorf("failed to extend role expiration: %w", err)
		}

		// Update the purchase record to link to the grant and record extension
		purchase.UserRoleGrantID = &grant.ID
		purchase.ExtensionDays = &durationDays
		if err := s.PurchaseRepo.Update(ctx, purchase); err != nil {
			return fmt.Errorf("failed to update purchase with grant link: %w", err)
		}

		log.WithFields(log.Fields{
			"userID":            userID,
			"roleID":            *product.RoleID,
			"purchaseID":        purchase.ID,
			"extensionDays":     durationDays,
			"newExpirationDate": newExpirationDate,
		}).Info("Extended role expiration via PayPal purchase")
	}

	return nil
}

// RevokeRoleGrant revokes a role grant (admin)
func (s *AdminSubscriptionService) RevokeRoleGrant(ctx context.Context, grantID uuid.UUID) error {
	return s.UserRoleGrantRepo.Delete(ctx, grantID)
}

// GetAllPurchases retrieves all purchases with filtering (admin)
func (s *AdminSubscriptionService) GetAllPurchases(ctx context.Context, queryOpts *query.QueryOptions[repo.GetPurchasesFilters]) ([]*models.Purchase, int64, error) {
	purchases, total, err := s.PurchaseRepo.GetPurchases(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get purchases: %w", err)
	}

	return purchases, total, nil
}

// GetAllNotifications retrieves all notifications with filtering (admin)
func (s *AdminSubscriptionService) GetAllNotifications(ctx context.Context, queryOpts *query.QueryOptions[repo.GetNotificationsFilters]) ([]*models.NotificationQueue, int64, error) {
	notifications, total, err := s.NotificationQueueRepo.GetNotifications(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get notifications: %w", err)
	}

	return notifications, total, nil
}

// SendManualNotification sends a manual notification (admin)
func (s *AdminSubscriptionService) SendManualNotification(ctx context.Context, userID uuid.UUID, eventType models.NotificationEventType, message string) error {
	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    userID,
		EventType: eventType,
		Data: map[string]any{
			"message": message,
			"source":  "admin_manual",
		},
	}

	return s.CreateNotification(ctx, notification)
}
