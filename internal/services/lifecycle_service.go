package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
	notificationService      *NotificationService
}

type CreateMembershipParams struct {
	UserID                  string
	PriceID                 uuid.UUID
	Processor               models.Processor
	ProcessorSubscriptionID *string
	UserEmail               *string
	ProcessorProvider       string
}

type RenewMembershipParams struct {
	Processor               models.Processor
	ProcessorSubscriptionID string
	ProcessorProvider       string
}

type CancelMembershipParams struct {
	SubscriptionID          *uuid.UUID
	Processor               *models.Processor
	ProcessorSubscriptionID *string
	CancelType              models.CancelType
	CancelFeedback          *string
	ImmediateCancellation   bool
	ProcessorProvider       string
}

type FailMembershipParams struct {
	Processor               models.Processor
	ProcessorSubscriptionID string
	ProcessorProvider       string
	FailureReason           *string
	FailureCode             *string
}

// NewSubscriptionLifecycleService creates a new instance of SubscriptionLifecycleService
func NewSubscriptionLifecycleService(db *db.DB, productService *ProductService, priceService *PriceService, entitlementService *EntitlementService, notificationService *NotificationQueueService) *SubscriptionLifecycleService {
	return &SubscriptionLifecycleService{
		DB:                       db,
		ProductService:           productService,
		PriceService:             priceService,
		EntitlementService:       entitlementService,
		NotificationQueueService: notificationService,
		notificationService:      nil,
	}
}

// SetNotificationService allows notification dispatch to run post-transaction
func (s *SubscriptionLifecycleService) SetNotificationService(notificationService *NotificationService) {
	s.notificationService = notificationService
}

func (s *SubscriptionLifecycleService) dispatchNotifications(ctx context.Context, notifications []*models.NotificationQueue) {
	if s.notificationService == nil {
		return
	}
	for _, notification := range notifications {
		if err := s.notificationService.DeliverEmail(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).WithFields(log.Fields{
				"notification_id": notification.ID,
				"event_type":      notification.EventType,
				"user_id":         notification.UserID,
			}).Error("failed to deliver notification email")
		}
	}
}

// CreateMembership creates a new subscription and grants associated roles
func (s *SubscriptionLifecycleService) CreateMembership(ctx context.Context, params *CreateMembershipParams) (*models.Subscription, error) {
	var (
		subscription  *models.Subscription
		notifications []*models.NotificationQueue
	)

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		dbb := db.NewWithTx(tx)
		var err error
		subscription, notifications, err = s.createMembershipCore(ctx, dbb, params)
		return err
	})
	if err != nil {
		return nil, err
	}

	s.dispatchNotifications(ctx, notifications)

	return subscription, nil
}

// CreateMembershipTx executes the membership creation logic using the provided transactional DB.
// The caller is responsible for wrapping the call in a transaction and dispatching any queued notifications.
func (s *SubscriptionLifecycleService) CreateMembershipTx(ctx context.Context, txDB *db.DB, params *CreateMembershipParams) (*models.Subscription, []*models.NotificationQueue, error) {
	if txDB == nil {
		return nil, nil, errors.New("transaction DB is required")
	}
	return s.createMembershipCore(ctx, txDB, params)
}

func (s *SubscriptionLifecycleService) createMembershipCore(ctx context.Context, dbb *db.DB, params *CreateMembershipParams) (*models.Subscription, []*models.NotificationQueue, error) {
	if dbb == nil {
		return nil, nil, errors.New("database handle is required")
	}

	priceService := NewPriceService(dbb)
	productService := NewProductService(dbb)
	entitlementService := NewEntitlementService(dbb)
	notificationService := NewNotificationQueueService(dbb)
	subService := NewSubscriptionService(dbb, priceService, productService, notificationService, nil, nil)

	price, err := priceService.GetByID(ctx, params.PriceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get price: %w", err)
	}

	existingSub, err := subService.GetByUserID(ctx, params.UserID)
	if err == nil && existingSub.Status == models.StatusActive {
		return nil, nil, fmt.Errorf("user already has an active subscription")
	}

	now := time.Now()
	periodStartsAt := now
	var periodEndsAt time.Time
	if price.BillingCycleDays != nil && *price.BillingCycleDays > 0 {
		periodEndsAt = now.Add(time.Duration(*price.BillingCycleDays) * 24 * time.Hour)
	} else {
		periodEndsAt = now.Add(30 * 24 * time.Hour)
	}

	var subscription *models.Subscription
	if existingSub != nil {
		if params.UserEmail != nil && strings.TrimSpace(*params.UserEmail) != "" {
			emailc := strings.TrimSpace(*params.UserEmail)
			existingSub.UserEmail = &emailc
		}

		existingSub.PriceID = price.ID
		existingSub.Status = models.StatusActive
		existingSub.Processor = params.Processor
		if params.ProcessorSubscriptionID != nil {
			existingSub.ProcessorSubscriptionID = *params.ProcessorSubscriptionID
		}

		if params.ProcessorProvider != "" {
			provider := params.ProcessorProvider
			existingSub.ProcessorProvider = &provider
		}

		existingSub.CurrentPeriodStartsAt = &periodStartsAt
		existingSub.CurrentPeriodEndsAt = &periodEndsAt
		existingSub.StartedAt = periodStartsAt
		existingSub.CancelledAt = nil
		existingSub.CancelType = nil
		existingSub.CancelFeedback = nil
		existingSub.EndedAt = nil

		if err := subService.Update(ctx, existingSub); err != nil {
			return nil, nil, fmt.Errorf("failed to update subscription: %w", err)
		}
		subscription = existingSub
	} else {
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

		if params.UserEmail != nil && strings.TrimSpace(*params.UserEmail) != "" {
			emailc := strings.TrimSpace(*params.UserEmail)
			subscription.UserEmail = &emailc
		}

		if params.ProcessorProvider != "" {
			provider := params.ProcessorProvider
			subscription.ProcessorProvider = &provider
		}

		if err := subService.Create(ctx, subscription); err != nil {
			return nil, nil, fmt.Errorf("failed to create subscription: %w", err)
		}
	}

	notifications := make([]*models.NotificationQueue, 0, 1)

	if entitlementService != nil {
		product, err := productService.GetByID(ctx, price.ProductID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get product: %w", err)
		}

		entNames := make([]string, 0, 4)
		if len(product.EntitlementsSpec) > 0 {
			for name := range product.EntitlementsSpec {
				entNames = append(entNames, name)
			}
		} else {
			entNames = append(entNames, "premium")
		}

		for _, ent := range entNames {
			exists, err := entitlementService.ExistsBySource(ctx, models.EntitlementSourceSubscription, subscription.ID, ent)
			if err != nil {
				return nil, nil, fmt.Errorf("failed entitlement check: %w", err)
			}
			if exists {
				continue
			}

			start := periodStartsAt
			finite, err := entitlementService.LatestFiniteWindow(ctx, params.UserID, ent, now)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, nil, fmt.Errorf("failed to fetch finite entitlement: %w", err)
			}
			if err == nil && finite != nil && finite.EndAt != nil {
				start = *finite.EndAt
			}

			if _, err := entitlementService.GrantWindow(ctx, params.UserID, ent, start, nil, models.EntitlementSourceSubscription, &subscription.ID); err != nil {
				return nil, nil, fmt.Errorf("failed to grant entitlement %s: %w", ent, err)
			}
		}
	}

	notification := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    params.UserID,
		EventType: models.NotificationPremiumStarted,
	}
	if err := notificationService.Create(ctx, notification); err != nil {
		log.WithContext(ctx).WithError(err).Error("failed to create membership started notification")
	} else {
		notifications = append(notifications, notification)
	}

	return subscription, notifications, nil
}

// RenewMembership renews an existing subscription and extends the membership
func (s *SubscriptionLifecycleService) RenewMembership(ctx context.Context, params *RenewMembershipParams) error {
	notifications := make([]*models.NotificationQueue, 0, 1)

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationQueueService := NewNotificationQueueService(db)
		subService := NewSubscriptionService(db, priceService, productService, notificationQueueService, nil, nil)

		// Find subscription
		provider := ""
		if params.Processor == models.ProcessorNMI {
			provider = strings.TrimSpace(strings.ToLower(params.ProcessorProvider))
			if provider == "" {
				provider = "mobius"
			}
		}

		subscription, err := subService.GetByProcessorSubscriptionID(ctx, string(params.Processor), provider, params.ProcessorSubscriptionID)
		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		if provider != "" {
			providerCopy := provider
			subscription.ProcessorProvider = &providerCopy
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

		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: models.NotificationPremiumRenewed,
		}
		if err := notificationQueueService.Create(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to create membership renewed notification")
		} else {
			notifications = append(notifications, notification)
		}

		return nil
	})

	if err != nil {
		return err
	}

	s.dispatchNotifications(ctx, notifications)

	return nil
}

// CancelMembership cancels a subscription and revokes associated roles
func (s *SubscriptionLifecycleService) CancelMembership(ctx context.Context, params *CancelMembershipParams) error {
	notifications := make([]*models.NotificationQueue, 0, 1)

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationQueueService := NewNotificationQueueService(db)
		subService := NewSubscriptionService(db, priceService, productService, notificationQueueService, nil, nil)
		entSvc := NewEntitlementService(db)

		provider := ""
		if params.Processor != nil && *params.Processor == models.ProcessorNMI {
			provider = strings.TrimSpace(strings.ToLower(params.ProcessorProvider))
			if provider == "" {
				provider = "mobius"
			}
		}

		// Find subscription
		var subscription *models.Subscription
		var err error

		if params.SubscriptionID != nil {
			subscription, err = subService.GetByID(ctx, *params.SubscriptionID)
		} else if params.ProcessorSubscriptionID != nil && params.Processor != nil {
			subscription, err = subService.GetByProcessorSubscriptionID(ctx, string(*params.Processor), provider, *params.ProcessorSubscriptionID)
		} else {
			return fmt.Errorf("either subscription_id or processor details must be provided")
		}

		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		if provider != "" {
			providerCopy := provider
			subscription.ProcessorProvider = &providerCopy
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

		reason := PremiumEndReasonAdmin
		switch params.CancelType {
		case models.CancelTypeUser:
			reason = PremiumEndReasonUserCancel
		case models.CancelTypeExpired:
			reason = PremiumEndReasonExpired
		case models.CancelTypeMerchant:
			reason = PremiumEndReasonProcessor
		}

		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: models.NotificationPremiumEnded,
			Data:      map[string]any{"reason": string(reason)},
		}
		if err := notificationQueueService.Create(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to create membership ended notification")
		} else {
			notifications = append(notifications, notification)
		}

		return nil
	})

	if err != nil {
		return err
	}

	s.dispatchNotifications(ctx, notifications)

	return nil
}

// ExpireMembership expires a subscription and revokes associated roles
func (s *SubscriptionLifecycleService) ExpireMembership(ctx context.Context, subscriptionID uuid.UUID) error {
	notifications := make([]*models.NotificationQueue, 0, 1)

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
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

		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: models.NotificationPremiumEnded,
			Data:      map[string]any{"reason": string(PremiumEndReasonExpired)},
		}
		if err := notificationQueueService.Create(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to create membership expired notification")
		} else {
			notifications = append(notifications, notification)
		}

		return nil
	})

	if err != nil {
		return err
	}

	s.dispatchNotifications(ctx, notifications)

	return nil
}

// FailMembership marks a subscription as failed due to payment issues
func (s *SubscriptionLifecycleService) FailMembership(ctx context.Context, params *FailMembershipParams) error {
	notifications := make([]*models.NotificationQueue, 0, 1)

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationQueueService := NewNotificationQueueService(db)
		subService := NewSubscriptionService(db, priceService, productService, notificationQueueService, nil, nil)
		entSvc := NewEntitlementService(db)

		provider := ""
		if params.Processor == models.ProcessorNMI {
			provider = strings.TrimSpace(strings.ToLower(params.ProcessorProvider))
			if provider == "" {
				provider = "mobius"
			}
		}

		subscription, err := subService.GetByProcessorSubscriptionID(ctx, string(params.Processor), provider, params.ProcessorSubscriptionID)
		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		if provider != "" {
			providerCopy := provider
			subscription.ProcessorProvider = &providerCopy
		}

		// Update subscription status - Wave 18: failed payment = past_due (still trying to recover)
		subscription.Status = models.StatusPastDue

		// Dunning policy (Mobius): try every 3 days, up to 5 failures total
		// Example timeline (D = day of initial failure): D+3, D+6, D+9, D+12, D+15
		now := time.Now()
		subscription.LastRetryAt = &now
		if subscription.RetryAttempts == nil {
			attempts := 1
			subscription.RetryAttempts = &attempts
		} else {
			*subscription.RetryAttempts++
		}

		// If we've reached MaxDunningFailures, cancel; otherwise schedule next attempt in DunningInterval
		if *subscription.RetryAttempts >= MaxDunningFailures {
			subscription.Status = models.StatusCancelled
			subscription.EndedAt = &now
		} else {
			nextRetry := now.Add(DunningInterval)
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

		eventType := models.NotificationPaymentMethodFailed
		if subscription.Status == models.StatusCancelled {
			eventType = models.NotificationPremiumEnded
		}

		var data map[string]any
		if eventType == models.NotificationPremiumEnded {
			data = map[string]any{"reason": string(PremiumEndReasonExpired)}
		}

		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: eventType,
			Data:      data,
		}
		if err := notificationQueueService.Create(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to create payment failed notification")
		} else {
			notifications = append(notifications, notification)
		}

		return nil
	})

	if err != nil {
		return err
	}

	s.dispatchNotifications(ctx, notifications)

	return nil
}

// Parameter structs for lifecycle operations
