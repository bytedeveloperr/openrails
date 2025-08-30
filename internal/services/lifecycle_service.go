package services

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/database"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

// SubscriptionLifecycleService handles the complete lifecycle of subscriptions
// including membership creation, renewal, cancellation, and expiration
type SubscriptionLifecycleService struct {
	DB *database.DB
}

// CreateMembership creates a new subscription and grants associated roles
func (s *SubscriptionLifecycleService) CreateMembership(ctx context.Context, params *CreateMembershipParams) (*models.Subscription, error) {
	var subscription *models.Subscription

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := database.NewWithTx(tx)
		subRepo := repo.NewSubscriptionRepo(db)
		userRoleGrantRepo := repo.NewUserRoleGrantRepo(db)
		notificationRepo := repo.NewNotificationQueueRepo(db)

		// Get price and product information
		price, err := s.PriceRepo.GetByID(ctx, params.PriceID)
		if err != nil {
			return fmt.Errorf("failed to get price: %w", err)
		}

		product, err := s.ProductRepo.GetByID(ctx, price.ProductID)
		if err != nil {
			return fmt.Errorf("failed to get product: %w", err)
		}

		// Check for existing active subscription
		existingSub, err := subRepo.GetByUserID(ctx, params.UserID)
		if err == nil && existingSub.Status == models.StatusActive {
			return fmt.Errorf("user already has an active subscription")
		}

		// Calculate billing period
		now := time.Now()
		periodStartsAt := now
		var periodEndsAt time.Time
		if price.BillingCycleDays != nil && *price.BillingCycleDays > 0 {
			periodEndsAt = now.Add(time.Duration(*price.BillingCycleDays) * 24 * time.Hour)
		} else {
			// Default to 30 days if no billing cycle specified
			periodEndsAt = now.Add(30 * 24 * time.Hour)
		}

		// Create or update subscription
		if existingSub != nil {
			// Update existing subscription
			existingSub.PriceID = price.ID
			existingSub.Status = models.StatusActive
			existingSub.Processor = params.Processor
			if params.ProcessorSubscriptionID != nil {
				existingSub.ProcessorSubscriptionID = *params.ProcessorSubscriptionID
			}
			// Transaction IDs now stored in Purchase table
			existingSub.CurrentPeriodStartsAt = &periodStartsAt
			existingSub.CurrentPeriodEndsAt = &periodEndsAt
			existingSub.StartedAt = periodStartsAt
			existingSub.CancelledAt = nil
			existingSub.CancelType = nil
			existingSub.CancelFeedback = nil
			existingSub.EndedAt = nil

			if err := subRepo.Update(ctx, existingSub); err != nil {
				return fmt.Errorf("failed to update subscription: %w", err)
			}
			subscription = existingSub
		} else {
			// Create new subscription
			subscription = &models.Subscription{
				ID:        uuid.New(),
				UserID:    params.UserID,
				PriceID:   price.ID,
				Status:    models.StatusActive,
				Processor: params.Processor,
				ProcessorSubscriptionID: func() string {
					if params.ProcessorSubscriptionID != nil {
						return *params.ProcessorSubscriptionID
					}
					return ""
				}(),
				CurrentPeriodStartsAt: &periodStartsAt,
				CurrentPeriodEndsAt:   &periodEndsAt,
				StartedAt:             periodStartsAt,
			}

			if err := subRepo.Create(ctx, subscription); err != nil {
				return fmt.Errorf("failed to create subscription: %w", err)
			}
		}

		// Grant role if product has one associated
		if product.RoleID != nil {
			if err := s.grantRole(ctx, userRoleGrantRepo, product, params.UserID, subscription.ID, params.GrantSource); err != nil {
				return fmt.Errorf("failed to grant role: %w", err)
			}
		}

		// Add notification
		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    params.UserID,
			EventType: models.NotificationPremiumStarted,
		}
		if err := notificationRepo.Create(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to create membership started notification")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return subscription, nil
}

// RenewMembership renews an existing subscription and extends the membership
func (s *SubscriptionLifecycleService) RenewMembership(ctx context.Context, params *RenewMembershipParams) error {
	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := database.NewWithTx(tx)
		subRepo := repo.NewSubscriptionRepo(db)
		userRoleGrantRepo := repo.NewUserRoleGrantRepo(db)
		notificationRepo := repo.NewNotificationQueueRepo(db)

		// Find subscription
		subscription, err := subRepo.GetByProcessorSubscriptionID(ctx, string(params.Processor), params.ProcessorSubscriptionID)
		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		// Get price for billing period calculation
		price, err := s.PriceRepo.GetByID(ctx, subscription.PriceID)
		if err != nil {
			return fmt.Errorf("failed to get price: %w", err)
		}

		product, err := s.ProductRepo.GetByID(ctx, price.ProductID)
		if err != nil {
			return fmt.Errorf("failed to get product: %w", err)
		}

		// Calculate new billing period
		var periodStartsAt, periodEndsAt time.Time
		if subscription.CurrentPeriodEndsAt != nil && !subscription.CurrentPeriodEndsAt.IsZero() {
			periodStartsAt = *subscription.CurrentPeriodEndsAt
			periodEndsAt = periodStartsAt.Add(time.Duration(*price.BillingCycleDays) * 24 * time.Hour)
		} else {
			now := time.Now()
			periodStartsAt = now
			periodEndsAt = now.Add(time.Duration(*price.BillingCycleDays) * 24 * time.Hour)
		}

		// Update subscription
		subscription.Status = models.StatusActive
		// Transaction IDs now stored in Purchase table
		subscription.CurrentPeriodStartsAt = &periodStartsAt
		subscription.CurrentPeriodEndsAt = &periodEndsAt
		subscription.CancelledAt = nil
		subscription.CancelType = nil
		subscription.CancelFeedback = nil
		subscription.EndedAt = nil

		if err := subRepo.Update(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// Ensure role grant is still active
		if product.RoleID != nil {
			if err := s.grantRole(ctx, userRoleGrantRepo, product, subscription.UserID, subscription.ID, params.GrantSource); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to ensure role grant for renewal")
			}
		}

		// Add notification
		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: models.NotificationPremiumRenewed,
		}
		if err := notificationRepo.Create(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to create membership renewed notification")
		}

		return nil
	})
}

// CancelMembership cancels a subscription and revokes associated roles
func (s *SubscriptionLifecycleService) CancelMembership(ctx context.Context, params *CancelMembershipParams) error {
	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := database.NewWithTx(tx)
		subRepo := repo.NewSubscriptionRepo(db)
		userRoleGrantRepo := repo.NewUserRoleGrantRepo(db)
		notificationRepo := repo.NewNotificationQueueRepo(db)

		// Find subscription
		var subscription *models.Subscription
		var err error

		if params.SubscriptionID != nil {
			subscription, err = subRepo.GetByID(ctx, *params.SubscriptionID)
		} else if params.ProcessorSubscriptionID != nil && params.Processor != nil {
			subscription, err = subRepo.GetByProcessorSubscriptionID(ctx, string(*params.Processor), *params.ProcessorSubscriptionID)
		} else {
			return fmt.Errorf("either subscription_id or processor details must be provided")
		}

		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		// Update subscription status
		now := time.Now()
		subscription.Status = models.StatusCancelled
		subscription.CancelledAt = &now
		subscription.CancelType = &params.CancelType
		if params.CancelFeedback != nil {
			subscription.CancelFeedback = params.CancelFeedback
		}

		// Set end date to now or current period end based on immediate cancellation
		if params.ImmediateCancellation {
			subscription.EndedAt = &now
			subscription.CurrentPeriodEndsAt = &now
		} else {
			// Let subscription run until current period ends
			if subscription.CurrentPeriodEndsAt == nil {
				subscription.EndedAt = &now
			}
		}

		if err := subRepo.Update(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// Revoke role grants if immediate cancellation or subscription already ended
		if params.ImmediateCancellation || (subscription.CurrentPeriodEndsAt != nil && subscription.CurrentPeriodEndsAt.Before(now)) {
			if err := userRoleGrantRepo.RevokeBySubSourceID(ctx, subscription.ID); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to revoke role grants for cancelled subscription")
			}
		}

		// Add notification
		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: models.NotificationPremiumEnded,
		}
		if err := notificationRepo.Create(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to create membership ended notification")
		}

		return nil
	})
}

// ExpireMembership expires a subscription and revokes associated roles
func (s *SubscriptionLifecycleService) ExpireMembership(ctx context.Context, subscriptionID uuid.UUID) error {
	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := database.NewWithTx(tx)
		subRepo := repo.NewSubscriptionRepo(db)
		userRoleGrantRepo := repo.NewUserRoleGrantRepo(db)
		notificationRepo := repo.NewNotificationQueueRepo(db)

		subscription, err := subRepo.GetByID(ctx, subscriptionID)
		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		// Update subscription status - Wave 18: expired = cancelled (never rebill again)
		now := time.Now()
		subscription.Status = models.StatusCancelled
		subscription.EndedAt = &now

		if err := subRepo.Update(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// Revoke role grants
		if err := userRoleGrantRepo.RevokeBySubSourceID(ctx, subscription.ID); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to revoke role grants for expired subscription")
		}

		// Add notification
		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: models.NotificationPremiumEnded,
		}
		if err := notificationRepo.Create(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to create membership expired notification")
		}

		return nil
	})
}

// FailMembership marks a subscription as failed due to payment issues
func (s *SubscriptionLifecycleService) FailMembership(ctx context.Context, params *FailMembershipParams) error {
	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := database.NewWithTx(tx)
		subRepo := repo.NewSubscriptionRepo(db)
		userRoleGrantRepo := repo.NewUserRoleGrantRepo(db)
		notificationRepo := repo.NewNotificationQueueRepo(db)

		subscription, err := subRepo.GetByProcessorSubscriptionID(ctx, string(params.Processor), params.ProcessorSubscriptionID)
		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		// Update subscription status - Wave 18: failed payment = past_due (still trying to recover)
		subscription.Status = models.StatusPastDue

		// Set up retry logic for manual rebilling
		now := time.Now()
		subscription.LastRetryAt = &now
		if subscription.RetryAttempts == nil {
			attempts := 1
			subscription.RetryAttempts = &attempts
		} else {
			*subscription.RetryAttempts++
		}

		// Calculate next retry time (exponential backoff: 1 day, 3 days, 7 days)
		var nextRetry time.Time
		switch *subscription.RetryAttempts {
		case 1:
			nextRetry = now.Add(24 * time.Hour)
		case 2:
			nextRetry = now.Add(72 * time.Hour) // 3 days
		case 3:
			nextRetry = now.Add(168 * time.Hour) // 7 days
		default:
			// After 3 attempts, stop trying - Wave 18: cancelled (never rebill again)
			subscription.Status = models.StatusCancelled
			subscription.EndedAt = &now
		}

		if subscription.Status == models.StatusPastDue {
			subscription.NextRetryAt = &nextRetry
		}

		if err := subRepo.Update(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// Revoke role grants if subscription is cancelled (after max retries)
		if subscription.Status == models.StatusCancelled {
			if err := userRoleGrantRepo.RevokeBySubSourceID(ctx, subscription.ID); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to revoke role grants for failed subscription")
			}
		}

		// Add notification
		eventType := models.NotificationPaymentMethodFailed
		if subscription.Status == models.StatusCancelled {
			eventType = models.NotificationPremiumEnded
		}

		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: eventType,
		}
		if err := notificationRepo.Create(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to create payment failed notification")
		}

		return nil
	})
}

// grantRole grants a role to a user for a subscription
func (s *SubscriptionLifecycleService) grantRole(ctx context.Context, userRoleGrantRepo *repo.UserRoleGrantRepo, product *models.Product, userID, subscriptionID uuid.UUID, grantSource models.GrantSource) error {
	// For subscription renewals, we need to get the price to determine extension
	// This assumes a subscription is being renewed
	subscriptionRepo := repo.NewSubscriptionRepo(s.DB)
	subscription, err := subscriptionRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	priceRepo := repo.NewPriceRepo(s.DB)
	price, err := priceRepo.GetByID(ctx, subscription.PriceID)
	if err != nil {
		return fmt.Errorf("failed to get price: %w", err)
	}

	// Determine extension days
	var extensionDays int
	if product.RoleDurationDays != nil && *product.RoleDurationDays > 0 {
		extensionDays = *product.RoleDurationDays
	} else if price.BillingCycleDays != nil && *price.BillingCycleDays > 0 {
		extensionDays = *price.BillingCycleDays
	} else {
		extensionDays = 30 // Default fallback
	}

	// Extend the role expiration
	grant, _, err := userRoleGrantRepo.ExtendRoleExpiration(ctx, userID, *product.RoleID, extensionDays)
	if err != nil {
		return fmt.Errorf("failed to extend role expiration: %w", err)
	}

	// Create Purchase event for this subscription payment
	purchase := &models.Purchase{
		ID:              uuid.New(),
		UserID:          userID,
		PriceID:         subscription.PriceID,
		UserRoleGrantID: &grant.ID,
		Processor:       models.ProcessorType(subscription.Processor), // Convert from subscription processor type
		TransactionID:   subscription.ProcessorSubscriptionID,         // Use subscription ID as transaction reference
		Amount:          price.Amount,
		Currency:        price.Currency,
		ExtensionDays:   &extensionDays,
		PurchasedAt:     time.Now(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	purchaseRepo := repo.NewPurchaseRepo(s.DB)
	if err := purchaseRepo.Create(ctx, purchase); err != nil {
		return fmt.Errorf("failed to create purchase event: %w", err)
	}

	log.WithContext(ctx).WithFields(log.Fields{
		"userID":         userID,
		"roleID":         *product.RoleID, // Dereference the pointer
		"subscriptionID": subscriptionID,
		"grantSource":    grantSource,
		"extensionDays":  extensionDays,
	}).Info("Granted role for subscription")

	return nil
}

// Parameter structs for lifecycle operations

type CreateMembershipParams struct {
	UserID                  uuid.UUID
	PriceID                 uuid.UUID
	Processor               models.ProcessorType
	ProcessorSubscriptionID *string
	GrantSource             models.GrantSource
}

type RenewMembershipParams struct {
	Processor               models.ProcessorType
	ProcessorSubscriptionID string
	GrantSource             models.GrantSource
}

type CancelMembershipParams struct {
	SubscriptionID          *uuid.UUID
	Processor               *models.ProcessorType
	ProcessorSubscriptionID *string
	CancelType              models.CancelType
	CancelFeedback          *string
	ImmediateCancellation   bool
}

type FailMembershipParams struct {
	Processor               models.ProcessorType
	ProcessorSubscriptionID string
	FailureReason           *string
	FailureCode             *string
}
