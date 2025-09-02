package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
	SubscriptionService      *SubscriptionService
	ProductService           *ProductService
	PriceService             *PriceService
	UserRoleGrantService     *UserRoleGrantService
	NotificationQueueService *NotificationQueueService
	PaymentService           *PaymentService
	UserService              *UserService
}

// AdminSubscriptionResponse represents a subscription with enriched admin data
type AdminSubscriptionResponse struct {
	*models.Subscription
	User    *types.User     `json:"user,omitempty"`
	Product *models.Product `json:"product,omitempty"`
	Price   *models.Price   `json:"price,omitempty"`
}

// GetAllSubscriptions retrieves all subscriptions with filtering (admin)
func (s *AdminSubscriptionService) GetAllSubscriptions(ctx context.Context, queryOpts *query.QueryOptions[GetSubscriptionsFilters]) ([]*AdminSubscriptionResponse, int64, error) {
	subscriptions, total, err := s.SubscriptionService.GetSubscribers(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get subscriptions: %w", err)
	}

	responses := make([]*AdminSubscriptionResponse, len(subscriptions))
	for i, sub := range subscriptions {
		responses[i] = &AdminSubscriptionResponse{
			Subscription: sub,
		}

		// Enrich with user data
		if user, err := s.UserService.GetGoTrueUserByID(ctx, sub.UserID); err == nil {
			responses[i].User = user
		}

		// Enrich with price and product data if available
		if price, err := s.PriceService.GetByID(ctx, sub.PriceID); err == nil {
			responses[i].Price = price

			if product, err := s.ProductService.GetByID(ctx, price.ProductID); err == nil {
				responses[i].Product = product
			}
		}
	}

	return responses, total, nil
}

// GetSubscriptionByID retrieves a specific subscription with full details (admin)
func (s *AdminSubscriptionService) GetSubscriptionByID(ctx context.Context, subscriptionID uuid.UUID) (*AdminSubscriptionResponse, error) {
	subscription, err := s.SubscriptionService.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("subscription not found: %w", err)
	}

	response := &AdminSubscriptionResponse{
		Subscription: subscription,
	}

	// Enrich with price and product data if available
	if price, err := s.PriceService.GetByID(ctx, subscription.PriceID); err == nil {
		response.Price = price

		if product, err := s.ProductService.GetByID(ctx, price.ProductID); err == nil {
			response.Product = product
		}
	}

	// Enrich with user data
	if user, err := s.UserService.GetGoTrueUserByID(ctx, subscription.UserID); err == nil {
		response.User = user
	}

	return response, nil
}

// UpdateSubscription updates a subscription (admin)
func (s *AdminSubscriptionService) UpdateSubscription(ctx context.Context, subscriptionID uuid.UUID, updates map[string]any) error {
	subscription, err := s.SubscriptionService.GetByID(ctx, subscriptionID)
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

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	return nil
}

// CancelSubscription cancels a subscription (admin)
func (s *AdminSubscriptionService) CancelSubscription(ctx context.Context, subscriptionID uuid.UUID, reason string) error {
	subscription, err := s.SubscriptionService.GetByID(ctx, subscriptionID)
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

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Revoke role grants
	if err := s.UserRoleGrantService.RevokeBySubSourceID(ctx, subscription.ID); err != nil {
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
	if err := s.NotificationQueueService.Create(ctx, notification); err != nil {
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
	subscription, err := s.SubscriptionService.GetByUserID(ctx, userID)
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

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Revoke role grants
	if err := s.UserRoleGrantService.RevokeBySubSourceID(ctx, subscription.ID); err != nil {
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
	if err := s.NotificationQueueService.Create(ctx, notification); err != nil {
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
	subscription, err := s.SubscriptionService.GetByID(ctx, subscriptionID)
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

	if err := s.SubscriptionService.Update(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Admin actions don't generate user notifications - this is purely for admin operations

	return nil
}

// CreateProduct creates a new product (admin)
func (s *AdminSubscriptionService) CreateProduct(ctx context.Context, product *models.Product) error {
	product.ID = uuid.New()
	return s.ProductService.Create(ctx, product)
}

// UpdateProduct updates a product (admin)
func (s *AdminSubscriptionService) UpdateProduct(ctx context.Context, productID uuid.UUID, updates map[string]any) error {
	product, err := s.ProductService.GetByID(ctx, productID)
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

	return s.ProductService.Update(ctx, product)
}

// CreatePrice creates a new price (admin)
func (s *AdminSubscriptionService) CreatePrice(ctx context.Context, price *models.Price) error {
	price.ID = uuid.New()
	return s.PriceService.Create(ctx, price)
}

// GetAllUserRoleGrants retrieves all user role grants with filtering (admin)
func (s *AdminSubscriptionService) GetAllUserRoleGrants(ctx context.Context, queryOpts *query.QueryOptions[GetUserRoleGrantsFilters]) ([]*models.UserRoleGrant, int64, error) {
	grants, total, err := s.UserRoleGrantService.GetUserRoleGrants(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get role grants: %w", err)
	}

	return grants, total, nil
}

// CreateManualRoleGrant creates a manual role grant (admin)
func (s *AdminSubscriptionService) CreateManualRoleGrant(ctx context.Context, userID, roleID uuid.UUID, durationDays *int) error {
	if durationDays == nil {
		// Permanent manual grant - create with no expiration
		grant, err := s.UserRoleGrantService.CreatePermanentGrant(ctx, userID, roleID)
		if err != nil {
			return err
		}
		// Record admin extension (0 days) for audit (Postgres SoR)
		_ = NewUserRoleGrantExtensionService(s.UserRoleGrantService.GetDB()).CreateAdmin(ctx, grant.ID, 0)
		return nil
	} else {
		// Temporary manual grant - extend existing or create new with expiration
		grant, _, err := s.UserRoleGrantService.ExtendRoleExpiration(ctx, userID, roleID, *durationDays)
		if err != nil {
			return err
		}
		// Record admin extension event
		_ = NewUserRoleGrantExtensionService(s.UserRoleGrantService.GetDB()).CreateAdmin(ctx, grant.ID, *durationDays)
		return nil
	}
}

// VerifyPayPalPurchase verifies a PayPal payment and grants the associated role (admin)
// This is used when admins manually verify PayPal payments outside the automated system
func (s *AdminSubscriptionService) VerifyPayPalPurchase(ctx context.Context, userID, priceID uuid.UUID, paypalTransactionID string) error {
	// Get price information
	price, err := s.PriceService.GetByID(ctx, priceID)
	if err != nil {
		return fmt.Errorf("failed to get price: %w", err)
	}

	// Get product information to determine role
	product, err := s.ProductService.GetByID(ctx, price.ProductID)
	if err != nil {
		return fmt.Errorf("failed to get product: %w", err)
	}

	// Create purchase record for audit trail
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
		grant, newExpirationDate, err := s.UserRoleGrantService.ExtendRoleExpiration(ctx, userID, *product.RoleID, durationDays)
		if err != nil {
			return fmt.Errorf("failed to extend role expiration: %w", err)
		}

		// Create purchase record with linkage to the grant
		purchase := &models.Payment{
			ID:              uuid.New(),
			UserID:          userID,
			PriceID:         priceID,
			Processor:       models.ProcessorPayPal,
			TransactionID:   paypalTransactionID,
			Amount:          price.Amount,
			Currency:        price.Currency,
			ExtensionDays:   &durationDays,
			UserRoleGrantID: &grant.ID,
			PurchasedAt:     time.Now(),
			CreatedAt:       time.Now(),
		}
		if err := s.PaymentService.Create(ctx, purchase); err != nil {
			return fmt.Errorf("failed to create purchase record: %w", err)
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
	return s.UserRoleGrantService.Delete(ctx, grantID)
}

// GetAllPurchases retrieves all purchases with filtering (admin)
func (s *AdminSubscriptionService) GetAllPurchases(ctx context.Context, queryOpts *query.QueryOptions[GetPaymentsFilters]) ([]*models.Payment, int64, error) {
	purchases, total, err := s.PaymentService.GetPayments(ctx, *queryOpts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get purchases: %w", err)
	}

	return purchases, total, nil
}

// GetAllNotifications retrieves all notifications with filtering (admin)
func (s *AdminSubscriptionService) GetAllNotifications(ctx context.Context, queryOpts *query.QueryOptions[GetNotificationsFilters]) ([]*models.NotificationQueue, int64, error) {
	notifications, total, err := s.NotificationQueueService.GetNotifications(ctx, *queryOpts)
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

	return s.NotificationQueueService.Create(ctx, notification)
}
