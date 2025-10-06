package services

import (
	"context"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db/models"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

// SubscriptionLifecycleService handles the complete lifecycle of subscriptions
// including membership creation, renewal, cancellation, and expiration
type SubscriptionLifecycleService struct {
	DB                       *db.DB
	ProductService           *ProductService
	PriceService             *PriceService
	EntitlementService       *EntitlementService
	NotificationQueueService *NotificationQueueService
}

// NewSubscriptionLifecycleService creates a new instance of SubscriptionLifecycleService
func NewSubscriptionLifecycleService(db *db.DB, productService *ProductService, priceService *PriceService, entitlementService *EntitlementService, notificationService *NotificationQueueService) *SubscriptionLifecycleService {
	return &SubscriptionLifecycleService{
		DB:                       db,
		ProductService:           productService,
		PriceService:             priceService,
		EntitlementService:       entitlementService,
		NotificationQueueService: notificationService,
	}
}

// CreateMembership creates a new subscription and grants associated roles
func (s *SubscriptionLifecycleService) CreateMembership(ctx context.Context, params *CreateMembershipParams) (*models.Subscription, error) {
	var subscription *models.Subscription

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		dbb := db.NewWithTx(tx)
		priceService := NewPriceService(dbb)
		productService := NewProductService(dbb)
		entitlementService := NewEntitlementService(dbb)
		notificationService := NewNotificationQueueService(dbb)
		subService := NewSubscriptionService(dbb, priceService, productService, notificationService, nil, nil)

		// Get price information
		price, err := s.PriceService.GetByID(ctx, params.PriceID)
		if err != nil {
			return fmt.Errorf("failed to get price: %w", err)
		}

		// Check for existing active subscription
		existingSub, err := subService.GetByUserID(ctx, params.UserID)
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
			if params.UserEmail != nil {
				existingSub.UserEmail = params.UserEmail
			}
			if params.Username != nil {
				existingSub.Username = params.Username
			}

			// Transaction IDs now stored in Purchase table
			existingSub.CurrentPeriodStartsAt = &periodStartsAt
			existingSub.CurrentPeriodEndsAt = &periodEndsAt
			existingSub.StartedAt = periodStartsAt
			existingSub.CancelledAt = nil
			existingSub.CancelType = nil
			existingSub.CancelFeedback = nil
			existingSub.EndedAt = nil

			if err := subService.Update(ctx, existingSub); err != nil {
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
				UserEmail:             params.UserEmail,
				Username:              params.Username,
			}

			if err := subService.Create(ctx, subscription); err != nil {
				return fmt.Errorf("failed to create subscription: %w", err)
			}
		}

		// Ensure subscription entitlements based on product EntitlementsSpec
		if entitlementService != nil {
			product, err := s.ProductService.GetByID(ctx, price.ProductID)
			if err != nil {
				return fmt.Errorf("failed to get product: %w", err)
			}

			// Build list of entitlement names
			entNames := make([]string, 0, 4)
			if len(product.EntitlementsSpec) > 0 {
				for name := range product.EntitlementsSpec {
					entNames = append(entNames, name)
				}
			} else {
				entNames = append(entNames, "premium")
			}

			// For each entitlement: create an indefinite window starting at period start,
			// aligned to end of any currently active finite window to avoid overlap.
			now := time.Now()
			for _, ent := range entNames {
				// Skip if this subscription already created this entitlement
				exists, _ := entitlementService.GetDB().GetDB().NewSelect().
					Model((*models.Entitlement)(nil)).
					Where("source_type = ? AND source_id = ? AND entitlement = ? AND revoked_at IS NULL", models.EntitlementSourceSubscription, subscription.ID, ent).
					Exists(ctx)
				if exists {
					continue
				}
				start := periodStartsAt
				var finite models.Entitlement
				_ = entitlementService.GetDB().GetDB().NewSelect().Model(&finite).
					Where("user_id = ? AND entitlement = ?", params.UserID, ent).
					Where("revoked_at IS NULL").
					Where("end_at IS NOT NULL").
					Where("start_at <= ?", now).
					Where("end_at > ?", now).
					Order("end_at DESC").
					Limit(1).
					Scan(ctx)
				if finite.ID != uuid.Nil && finite.EndAt != nil {
					start = *finite.EndAt
				}
				_, _ = entitlementService.GrantWindow(ctx, params.UserID, ent, start, nil, models.EntitlementSourceSubscription, &subscription.ID)
			}
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
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationQueueService := NewNotificationQueueService(db)
		subService := NewSubscriptionService(db, priceService, productService, notificationQueueService, nil, nil)

		// Find subscription
		subscription, err := subService.GetByProcessorSubscriptionID(ctx, string(params.Processor), params.ProcessorSubscriptionID)
		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		// Get price for billing period calculation
		price, err := s.PriceService.GetByID(ctx, subscription.PriceID)
		if err != nil {
			return fmt.Errorf("failed to get price: %w", err)
		}

		_ = price // already used below

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

		if err := subService.Update(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// On renewal: no new subscription entitlement needed; the open window remains.

		return nil
	})
}

// CancelMembership cancels a subscription and revokes associated roles
func (s *SubscriptionLifecycleService) CancelMembership(ctx context.Context, params *CancelMembershipParams) error {
	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationQueueService := NewNotificationQueueService(db)
		subService := NewSubscriptionService(db, priceService, productService, notificationQueueService, nil, nil)
		entSvc := NewEntitlementService(db)

		// Find subscription
		var subscription *models.Subscription
		var err error

		if params.SubscriptionID != nil {
			subscription, err = subService.GetByID(ctx, *params.SubscriptionID)
		} else if params.ProcessorSubscriptionID != nil && params.Processor != nil {
			subscription, err = subService.GetByProcessorSubscriptionID(ctx, string(*params.Processor), *params.ProcessorSubscriptionID)
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

		if err := subService.Update(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// End entitlements at correct boundary: immediate or at period end
		if params.ImmediateCancellation || (subscription.CurrentPeriodEndsAt != nil && subscription.CurrentPeriodEndsAt.Before(now)) {
			if entSvc != nil {
				endAt := now
				if !params.ImmediateCancellation && subscription.CurrentPeriodEndsAt != nil {
					endAt = *subscription.CurrentPeriodEndsAt
				}
				reason := models.EntitlementRevokeAdmin
				if err := entSvc.EndActiveBySubscription(ctx, subscription.ID, endAt, &reason); err != nil {
					log.WithContext(ctx).WithError(err).Error("failed to revoke entitlements for cancelled subscription")
				}
			}
		}

		return nil
	})
}

// ExpireMembership expires a subscription and revokes associated roles
func (s *SubscriptionLifecycleService) ExpireMembership(ctx context.Context, subscriptionID uuid.UUID) error {
	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationQueueService := NewNotificationQueueService(db)
		subService := NewSubscriptionService(db, priceService, productService, notificationQueueService, nil, nil)
		entSvc := NewEntitlementService(db)

		subscription, err := subService.GetByID(ctx, subscriptionID)
		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		// Update subscription status - Wave 18: expired = cancelled (never rebill again)
		now := time.Now()
		subscription.Status = models.StatusCancelled
		subscription.EndedAt = &now

		if err := subService.Update(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// Revoke entitlements
		if entSvc != nil {
			reason := models.EntitlementRevokeAdmin
			if err := entSvc.EndActiveBySubscription(ctx, subscription.ID, now, &reason); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to revoke entitlements for expired subscription")
			}
		}

		return nil
	})
}

// FailMembership marks a subscription as failed due to payment issues
func (s *SubscriptionLifecycleService) FailMembership(ctx context.Context, params *FailMembershipParams) error {
	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationQueueService := NewNotificationQueueService(db)
		subService := NewSubscriptionService(db, priceService, productService, notificationQueueService, nil, nil)
		entSvc := NewEntitlementService(db)

		subscription, err := subService.GetByProcessorSubscriptionID(ctx, string(params.Processor), params.ProcessorSubscriptionID)
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

		if err := subService.Update(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// Revoke entitlements if subscription is cancelled (after max retries)
		if subscription.Status == models.StatusCancelled {
			if entSvc != nil {
				reason := models.EntitlementRevokeAdmin
				if err := entSvc.EndActiveBySubscription(ctx, subscription.ID, now, &reason); err != nil {
					log.WithContext(ctx).WithError(err).Error("failed to revoke entitlements for failed subscription")
				}
			}
		}

		return nil
	})
}

// deprecated grantRole removed; entitlements managed by subscription lifecycle

// Parameter structs for lifecycle operations

type CreateMembershipParams struct {
	UserID                  string
	PriceID                 uuid.UUID
	Processor               models.Processor
	ProcessorSubscriptionID *string
	UserEmail               *string
	Username                *string
}

type RenewMembershipParams struct {
	Processor               models.Processor
	ProcessorSubscriptionID string
}

type CancelMembershipParams struct {
	SubscriptionID          *uuid.UUID
	Processor               *models.Processor
	ProcessorSubscriptionID *string
	CancelType              models.CancelType
	CancelFeedback          *string
	ImmediateCancellation   bool
}

type FailMembershipParams struct {
	Processor               models.Processor
	ProcessorSubscriptionID string
	FailureReason           *string
	FailureCode             *string
}
