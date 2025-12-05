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
	"github.com/doujins-org/doujins-billing/internal/processors"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

// SubscriptionLifecycleService handles the complete lifecycle of subscriptions
// including membership creation, renewal, cancellation, and expiration
type SubscriptionLifecycleService struct {
	DB                  *db.DB
	Clock               clockwork.Clock
	ProductService      *ProductService
	PriceService        *PriceService
	EntitlementService  *EntitlementService
	NotificationService *NotificationService
	PaymentService      *PaymentService   // For creating Payment records on renewal
	EventLogService     *EventLogService  // For logging events to ClickHouse
}

type CreateMembershipParams struct {
	UserID                  string
	PriceID                 uuid.UUID
	Processor               models.Processor
	ProcessorSubscriptionID *string
	UserEmail               *string
	// Payment fields - required for creating Payment record
	TransactionID string // Processor's transaction ID for this purchase
	Amount        int64  // Amount charged in smallest unit (cents for USD)
	Currency      string // Currency code (e.g., "usd")
}

type RenewMembershipParams struct {
	Processor               models.Processor
	ProcessorSubscriptionID string
	// Payment fields - required for creating Payment record
	TransactionID string // Processor's transaction ID for this renewal
	Amount        int64  // Amount charged in smallest unit (cents for USD)
	Currency      string // Currency code (e.g., "usd")
}

type CancelMembershipParams struct {
	SubscriptionID          *uuid.UUID
	Processor               *models.Processor
	ProcessorSubscriptionID *string
	CancelType              models.CancelType
	CancelFeedback          *string
	RevokeAccess            bool // If true, entitlements revoked immediately. If false, access continues until period end.
}

type FailMembershipParams struct {
	Processor               models.Processor
	ProcessorSubscriptionID string
	FailureReason           *string
	FailureCode             *string
}

// NewSubscriptionLifecycleService creates a new instance of SubscriptionLifecycleService
func NewSubscriptionLifecycleService(db *db.DB, productService *ProductService, priceService *PriceService, entitlementService *EntitlementService, notificationService *NotificationService, paymentService *PaymentService, eventLogService *EventLogService) *SubscriptionLifecycleService {
	return &SubscriptionLifecycleService{
		DB:                  db,
		Clock:               clockwork.NewRealClock(), // Default to real clock, can be overridden for tests
		ProductService:      productService,
		PriceService:        priceService,
		EntitlementService:  entitlementService,
		NotificationService: notificationService,
		PaymentService:      paymentService,
		EventLogService:     eventLogService,
	}
}

// SetClock allows replacing the clock for testing
func (s *SubscriptionLifecycleService) SetClock(c clockwork.Clock) {
	s.Clock = c
}

// now returns the current time from the service's clock
func (s *SubscriptionLifecycleService) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now()
	}
	return time.Now()
}

func (s *SubscriptionLifecycleService) dispatchNotifications(ctx context.Context, notifications []*models.NotificationQueue) {
	if s.NotificationService == nil {
		return
	}
	for _, notification := range notifications {
		if err := s.NotificationService.DeliverEmail(ctx, notification); err != nil {
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

	// Log the charge success event to ClickHouse
	s.logPaymentEvent(ctx, subscription, params.Processor, params.TransactionID, params.Amount, params.Currency, PaymentEventChargeSuccess)

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
	entitlementService.SetClock(s.Clock) // Propagate clock for testing
	notificationService := NewNotificationService(dbb, nil)
	subService := NewSubscriptionService(dbb, priceService, productService, notificationService, nil, nil, nil)

	price, err := priceService.GetByID(ctx, params.PriceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get price: %w", err)
	}

	existingSub, err := subService.GetByUserID(ctx, params.UserID)
	if err == nil && existingSub.Status == models.StatusActive {
		return nil, nil, fmt.Errorf("user already has an active subscription")
	}

	now := s.now()
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
			ProductID: price.ProductID,
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

	// Create Payment record if payment info is provided
	if params.TransactionID != "" && s.PaymentService != nil {
		paymentService := NewPaymentService(dbb)

		// Use provided amount/currency or fall back to price defaults
		amount := params.Amount
		if amount == 0 {
			amount = price.Amount
		}
		currency := params.Currency
		if currency == "" {
			currency = price.Currency
		}

		payment := &models.Payment{
			ID:             uuid.New(),
			UserID:         params.UserID,
			PriceID:        price.ID,
			SubscriptionID: &subscription.ID,
			Processor:      params.Processor,
			TransactionID:  params.TransactionID,
			Amount:         amount,
			Currency:       currency,
			PurchasedAt:    now,
			CreatedAt:      now,
		}
		if err := paymentService.Create(ctx, payment); err != nil {
			log.WithContext(ctx).WithError(err).WithFields(log.Fields{
				"transaction_id":  params.TransactionID,
				"subscription_id": subscription.ID,
				"user_id":         params.UserID,
			}).Error("failed to create payment record for new membership")
			// Don't fail the membership creation - just log the error
		}
	}

	return subscription, notifications, nil
}

// RenewMembership renews an existing subscription and extends the membership.
// It also creates a Payment record for the renewal transaction.
// If a scheduled downgrade exists (ScheduledPriceID), it will be applied on renewal.
func (s *SubscriptionLifecycleService) RenewMembership(ctx context.Context, params *RenewMembershipParams) error {
	notifications := make([]*models.NotificationQueue, 0, 1)

	// Variables to capture from transaction for Payment creation
	var subscriptionID uuid.UUID
	var userID string
	var priceID uuid.UUID

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationService := NewNotificationService(db, nil)
		subService := NewSubscriptionService(db, priceService, productService, notificationService, nil, nil, nil)
		entitlementService := NewEntitlementService(db)
		entitlementService.SetClock(s.Clock)

		// Find subscription - use processor name for gateway lookup
		provider := ""
		if processors.IsNMIBackedProcessor(params.Processor) {
			provider = strings.ToLower(string(params.Processor))
			if provider == "nmi" {
				provider = "mobius"
			}
		}

		subscription, err := subService.GetByProcessorSubscriptionID(ctx, string(params.Processor), provider, params.ProcessorSubscriptionID)
		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		// Capture values for Payment creation after transaction
		subscriptionID = subscription.ID
		userID = subscription.UserID

		// Check for scheduled downgrade
		var price *models.Price
		var oldProduct, newProduct *models.Product
		applyingDowngrade := subscription.ScheduledPriceID != nil

		if applyingDowngrade {
			// Get old product for entitlement comparison
			oldPrice, err := priceService.GetByID(ctx, subscription.PriceID)
			if err != nil {
				return fmt.Errorf("failed to get current price: %w", err)
			}
			oldProduct, err = productService.GetByID(ctx, oldPrice.ProductID)
			if err != nil {
				return fmt.Errorf("failed to get current product: %w", err)
			}

			// Apply the scheduled downgrade - switch to the new price
			price, err = priceService.GetByID(ctx, *subscription.ScheduledPriceID)
			if err != nil {
				return fmt.Errorf("failed to get scheduled price: %w", err)
			}

			newProduct, err = productService.GetByID(ctx, price.ProductID)
			if err != nil {
				return fmt.Errorf("failed to get new product: %w", err)
			}

			log.WithContext(ctx).WithFields(log.Fields{
				"subscription_id":    subscription.ID,
				"user_id":            subscription.UserID,
				"old_price_id":       subscription.PriceID,
				"new_price_id":       price.ID,
				"old_product":        oldProduct.DisplayName,
				"new_product":        newProduct.DisplayName,
			}).Info("Applying scheduled downgrade on renewal")

			// Update subscription to new price and product
			subscription.PriceID = price.ID
			subscription.ProductID = price.ProductID
			subscription.ScheduledPriceID = nil // Clear the scheduled downgrade
		} else {
			// Normal renewal - use current price
			price, err = priceService.GetByID(ctx, subscription.PriceID)
			if err != nil {
				return fmt.Errorf("failed to get price: %w", err)
			}
		}

		// Capture for Payment record
		priceID = price.ID

		// Calculate new billing period
		var periodStartsAt, periodEndsAt time.Time
		if subscription.CurrentPeriodEndsAt != nil && !subscription.CurrentPeriodEndsAt.IsZero() {
			periodStartsAt = *subscription.CurrentPeriodEndsAt
			periodEndsAt = periodStartsAt.Add(time.Duration(*price.BillingCycleDays) * 24 * time.Hour)
		} else {
			now := s.now()
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

		// Handle entitlements for downgrade
		if applyingDowngrade && oldProduct != nil && newProduct != nil {
			now := s.now()

			// Determine which entitlements to revoke (in old but not in new)
			oldEnts := make(map[string]bool)
			for name := range oldProduct.EntitlementsSpec {
				oldEnts[name] = true
			}

			newEnts := make(map[string]bool)
			for name := range newProduct.EntitlementsSpec {
				newEnts[name] = true
			}

			// Revoke entitlements that are in old product but not in new product
			for entName := range oldEnts {
				if !newEnts[entName] {
					reason := models.EntitlementRevokeDowngrade
					if err := entitlementService.RevokeBySubscriptionAndName(ctx, subscription.ID, entName, now, reason); err != nil {
						log.WithContext(ctx).WithError(err).WithFields(log.Fields{
							"subscription_id": subscription.ID,
							"entitlement":     entName,
						}).Warn("Failed to revoke entitlement during downgrade")
					} else {
						log.WithContext(ctx).WithFields(log.Fields{
							"subscription_id": subscription.ID,
							"entitlement":     entName,
						}).Info("Revoked entitlement due to downgrade")
					}
				}
			}

			// Grant any new entitlements that are in new product but not in old
			// (rare for downgrade, but handle for completeness)
			for entName := range newEnts {
				if !oldEnts[entName] {
					exists, err := entitlementService.ExistsBySource(ctx, models.EntitlementSourceSubscription, subscription.ID, entName)
					if err != nil {
						log.WithContext(ctx).WithError(err).WithField("entitlement", entName).Warn("Failed to check entitlement existence")
						continue
					}
					if !exists {
						if _, err := entitlementService.GrantWindow(ctx, subscription.UserID, entName, now, nil, models.EntitlementSourceSubscription, &subscription.ID); err != nil {
							log.WithContext(ctx).WithError(err).WithField("entitlement", entName).Warn("Failed to grant new entitlement during downgrade")
						}
					}
				}
			}
		}

		// Notify user
		eventType := models.NotificationPremiumRenewed
		var notifData map[string]any
		if applyingDowngrade && newProduct != nil {
			notifData = map[string]any{
				"downgrade_applied": true,
				"new_product":       newProduct.DisplayName,
			}
		}

		notification := &models.NotificationQueue{
			ID:        uuid.New(),
			UserID:    subscription.UserID,
			EventType: eventType,
			Data:      notifData,
		}
		if err := notificationService.Create(ctx, notification); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to create membership renewed notification")
		} else {
			notifications = append(notifications, notification)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Create Payment record for the renewal (outside transaction, non-fatal if fails)
	if s.PaymentService != nil && params.TransactionID != "" && params.Amount > 0 {
		// Check for existing payment to prevent duplicates
		existing, err := s.PaymentService.GetByTransactionID(ctx, params.Processor, params.TransactionID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.WithContext(ctx).WithError(err).WithField("transaction_id", params.TransactionID).
				Warn("Failed to check existing payment for renewal")
		}
		if existing == nil {
			now := s.now()
			payment := &models.Payment{
				ID:             uuid.New(),
				UserID:         userID,
				PriceID:        priceID,
				SubscriptionID: &subscriptionID,
				Processor:      params.Processor,
				TransactionID:  params.TransactionID,
				Amount:         params.Amount,
				Currency:       params.Currency,
				PurchasedAt:    now.UTC(),
				CreatedAt:      now.UTC(),
			}
			if err := s.PaymentService.Create(ctx, payment); err != nil {
				log.WithContext(ctx).WithError(err).WithFields(log.Fields{
					"transaction_id":  params.TransactionID,
					"subscription_id": subscriptionID,
					"user_id":         userID,
				}).Error("Failed to create payment record for renewal")
				// Don't fail - renewal was processed successfully
			}
		} else {
			log.WithContext(ctx).WithField("transaction_id", params.TransactionID).
				Debug("Renewal payment already recorded; skipping duplicate entry")
		}
	}

	s.dispatchNotifications(ctx, notifications)

	// Log the charge success event to ClickHouse
	if s.EventLogService != nil {
		var amountFloat *float64
		if params.Amount > 0 {
			f := float64(params.Amount) / 100.0
			amountFloat = &f
		}
		var txnID *string
		if params.TransactionID != "" {
			txnID = &params.TransactionID
		}
		currency := params.Currency
		if currency == "" {
			currency = "usd"
		}
		data := PaymentEventData{
			SubscriptionID:         &subscriptionID,
			UserID:                 userID,
			EventType:              PaymentEventChargeSuccess,
			Processor:              string(params.Processor),
			ProcessorTransactionID: txnID,
			Amount:                 amountFloat,
			Currency:               currency,
			BillingInfo:            "{}",
			WebhookSource:          "lifecycle",
			Metadata:               CreateMetadataJSON(map[string]interface{}{"subscription_id": subscriptionID.String(), "renewal": true}),
			Timestamp:              s.now().UTC(),
		}
		if err := s.EventLogService.LogPaymentEvent(ctx, data); err != nil {
			log.WithContext(ctx).WithError(err).WithFields(log.Fields{
				"subscription_id": subscriptionID,
				"event_type":      PaymentEventChargeSuccess,
			}).Warn("failed to log renewal payment event to ClickHouse")
		}
	}

	return nil
}

// CancelMembership cancels a subscription and revokes associated roles
func (s *SubscriptionLifecycleService) CancelMembership(ctx context.Context, params *CancelMembershipParams) error {
	notifications := make([]*models.NotificationQueue, 0, 1)

	// Variables to capture from transaction for event logging
	var subscriptionID uuid.UUID
	var userID string
	var processor models.Processor

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationService := NewNotificationService(db, nil)
		subService := NewSubscriptionService(db, priceService, productService, notificationService, nil, nil, nil)
		entSvc := NewEntitlementService(db)
		entSvc.SetClock(s.Clock) // Propagate clock for testing

		// Use processor name for gateway lookup
		provider := ""
		if params.Processor != nil && processors.IsNMIBackedProcessor(*params.Processor) {
			provider = strings.ToLower(string(*params.Processor))
			if provider == "nmi" {
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

		// Capture values for event logging after transaction
		subscriptionID = subscription.ID
		userID = subscription.UserID
		processor = subscription.Processor

		// Update subscription status
		now := s.now()
		subscription.Status = models.StatusCancelled
		subscription.CancelledAt = &now
		subscription.CancelType = &params.CancelType
		if params.CancelFeedback != nil {
			subscription.CancelFeedback = params.CancelFeedback
		}

		// Set end date to now or current period end based on whether access is revoked
		if params.RevokeAccess {
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
		// When RevokeAccess is true: end immediately with revocation reason
		// When RevokeAccess is false: set end_at to period end (entitlement remains active until then)
		if entSvc != nil {
			if params.RevokeAccess {
				// Immediate revocation - set end_at to now with a revocation reason
				reason := models.EntitlementRevokeAdmin
				if err := entSvc.EndActiveBySubscription(ctx, subscription.ID, now, &reason); err != nil {
					log.WithContext(ctx).WithError(err).Error("failed to revoke entitlements for cancelled subscription")
				}
			} else if subscription.CurrentPeriodEndsAt != nil && subscription.CurrentPeriodEndsAt.After(now) {
				// Period-end cancellation - just set end_at without revocation (user keeps access until then)
				if err := entSvc.EndActiveBySubscription(ctx, subscription.ID, *subscription.CurrentPeriodEndsAt, nil); err != nil {
					log.WithContext(ctx).WithError(err).Error("failed to set entitlement end date for cancelled subscription")
				}
			} else {
				// Period already ended - immediately revoke
				reason := models.EntitlementRevokeAdmin
				if err := entSvc.EndActiveBySubscription(ctx, subscription.ID, now, &reason); err != nil {
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
		if err := notificationService.Create(ctx, notification); err != nil {
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

	// Log the subscription cancelled event to ClickHouse
	if s.EventLogService != nil {
		data := SubscriptionEventData{
			SubscriptionID: subscriptionID,
			UserID:         userID,
			EventType:      PaymentEventSubscriptionCancelled,
			Processor:      string(processor),
			Metadata:       CreateMetadataJSON(map[string]interface{}{"cancel_type": string(params.CancelType), "revoke_access": params.RevokeAccess}),
			Timestamp:      s.now().UTC(),
		}
		if err := s.EventLogService.LogSubscriptionEvent(ctx, data); err != nil {
			log.WithContext(ctx).WithError(err).WithFields(log.Fields{
				"subscription_id": subscriptionID,
				"event_type":      PaymentEventSubscriptionCancelled,
			}).Warn("failed to log subscription cancelled event to ClickHouse")
		}
	}

	return nil
}

// ExpireMembership expires a subscription and revokes associated roles
func (s *SubscriptionLifecycleService) ExpireMembership(ctx context.Context, subscriptionID uuid.UUID) error {
	notifications := make([]*models.NotificationQueue, 0, 1)

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationService := NewNotificationService(db, nil)
		subService := NewSubscriptionService(db, priceService, productService, notificationService, nil, nil, nil)
		entSvc := NewEntitlementService(db)
		entSvc.SetClock(s.Clock) // Propagate clock for testing

		subscription, err := subService.GetByID(ctx, subscriptionID)
		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		// Update subscription status - Wave 18: expired = cancelled (never rebill again)
		now := s.now()
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
		if err := notificationService.Create(ctx, notification); err != nil {
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

	// Variables to capture from transaction for event logging
	var subscriptionID uuid.UUID
	var userID string
	var finalStatus models.SubscriptionStatus

	err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationService := NewNotificationService(db, nil)
		subService := NewSubscriptionService(db, priceService, productService, notificationService, nil, nil, nil)
		entSvc := NewEntitlementService(db)
		entSvc.SetClock(s.Clock) // Propagate clock for testing

		// Use processor name for gateway lookup
		provider := ""
		if processors.IsNMIBackedProcessor(params.Processor) {
			provider = strings.ToLower(string(params.Processor))
			if provider == "nmi" {
				provider = "mobius"
			}
		}

		subscription, err := subService.GetByProcessorSubscriptionID(ctx, string(params.Processor), provider, params.ProcessorSubscriptionID)
		if err != nil {
			return fmt.Errorf("subscription not found: %w", err)
		}

		// Capture values for event logging
		subscriptionID = subscription.ID
		userID = subscription.UserID

		// Update subscription status - Wave 18: failed payment = past_due (still trying to recover)
		subscription.Status = models.StatusPastDue

		// Dunning policy (Mobius): try every 3 days, up to 5 failures total
		// Example timeline (D = day of initial failure): D+3, D+6, D+9, D+12, D+15
		now := s.now()
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

		// Capture final status for event logging
		finalStatus = subscription.Status

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
		if err := notificationService.Create(ctx, notification); err != nil {
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

	// Log the payment failure event to ClickHouse
	if s.EventLogService != nil {
		eventType := PaymentEventChargeFailure
		metadata := map[string]interface{}{
			"subscription_id": subscriptionID.String(),
			"final_status":    string(finalStatus),
		}
		if params.FailureReason != nil {
			metadata["failure_reason"] = *params.FailureReason
		}
		if params.FailureCode != nil {
			metadata["failure_code"] = *params.FailureCode
		}

		// If the subscription was cancelled due to max retries, also log expiration
		if finalStatus == models.StatusCancelled {
			eventType = PaymentEventSubscriptionExpired
		}

		data := PaymentEventData{
			SubscriptionID: &subscriptionID,
			UserID:         userID,
			EventType:      eventType,
			Processor:      string(params.Processor),
			Currency:       "usd",
			BillingInfo:    "{}",
			WebhookSource:  "lifecycle",
			Metadata:       CreateMetadataJSON(metadata),
			Timestamp:      s.now().UTC(),
		}
		if err := s.EventLogService.LogPaymentEvent(ctx, data); err != nil {
			log.WithContext(ctx).WithError(err).WithFields(log.Fields{
				"subscription_id": subscriptionID,
				"event_type":      eventType,
			}).Warn("failed to log payment failure event to ClickHouse")
		}
	}

	return nil
}

// logPaymentEvent logs a payment event to ClickHouse via EventLogService.
// It's a helper that creates PaymentEventData from subscription and payment info.
func (s *SubscriptionLifecycleService) logPaymentEvent(ctx context.Context, sub *models.Subscription, processor models.Processor, transactionID string, amount int64, currency string, eventType PaymentEventType) {
	if s.EventLogService == nil || sub == nil {
		return
	}

	// Convert amount from cents to dollars for ClickHouse
	var amountFloat *float64
	if amount > 0 {
		f := float64(amount) / 100.0
		amountFloat = &f
	}

	var txnID *string
	if transactionID != "" {
		txnID = &transactionID
	}

	if currency == "" {
		currency = "usd"
	}

	data := PaymentEventData{
		SubscriptionID:         &sub.ID,
		UserID:                 sub.UserID,
		EventType:              eventType,
		Processor:              string(processor),
		ProcessorTransactionID: txnID,
		Amount:                 amountFloat,
		Currency:               currency,
		BillingInfo:            "{}",
		WebhookSource:          "lifecycle",
		Metadata:               CreateMetadataJSON(map[string]interface{}{"subscription_id": sub.ID.String()}),
		Timestamp:              s.now().UTC(),
	}

	if err := s.EventLogService.LogPaymentEvent(ctx, data); err != nil {
		log.WithContext(ctx).WithError(err).WithFields(log.Fields{
			"subscription_id": sub.ID,
			"event_type":      eventType,
			"processor":       processor,
		}).Warn("failed to log payment event to ClickHouse")
	}
}

// logSubscriptionEvent logs a subscription lifecycle event to ClickHouse.
func (s *SubscriptionLifecycleService) logSubscriptionEvent(ctx context.Context, sub *models.Subscription, processor models.Processor, eventType PaymentEventType, metadata map[string]interface{}) {
	if s.EventLogService == nil || sub == nil {
		return
	}

	var procSubID *string
	if sub.ProcessorSubscriptionID != "" {
		procSubID = &sub.ProcessorSubscriptionID
	}

	data := SubscriptionEventData{
		SubscriptionID:          sub.ID,
		UserID:                  sub.UserID,
		EventType:               eventType,
		Processor:               string(processor),
		ProcessorSubscriptionID: procSubID,
		Metadata:                CreateMetadataJSON(metadata),
		Timestamp:               s.now().UTC(),
	}

	if err := s.EventLogService.LogSubscriptionEvent(ctx, data); err != nil {
		log.WithContext(ctx).WithError(err).WithFields(log.Fields{
			"subscription_id": sub.ID,
			"event_type":      eventType,
			"processor":       processor,
		}).Warn("failed to log subscription event to ClickHouse")
	}
}

// Parameter structs for lifecycle operations
