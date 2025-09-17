package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db/models"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/integrations/mobius"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

const MobiusProcessorName string = "Mobius"

type MobiusWebhookService struct {
	DB                       *db.DB
	PriceService             *PriceService
	ProductService           *ProductService
	Data                     MobiusWebhookEvent
	MobiusClient             *mobius.MobiusClient
	DeadLetterService        *DeadLetterService
	NotificationQueueService *NotificationQueueService
	NotificationService      *NotificationService
	BillingEventService      *BillingEventService
	DeduplicationService     *DeduplicationService
}

type MobiusWebhookEventType = string

const (
	// Subscription lifecycle events
	EventTypeMobiusAddSubscription    MobiusWebhookEventType = "recurring.subscription.add"
	EventTypeMobiusUpdateSubscription MobiusWebhookEventType = "recurring.subscription.update"
	EventTypeMobiusDeleteSubscription MobiusWebhookEventType = "recurring.subscription.delete"

	// Transaction events
	EventTypeMobiusTransactionSuccess MobiusWebhookEventType = "transaction.sale.success"

	// Automatic Card Updater (ACU) events
	EventTypeMobiusACUUpdated         MobiusWebhookEventType = "acu.summary.automaticallyupdated"
	EventTypeMobiusACUContactCustomer MobiusWebhookEventType = "acu.summary.contactcustomer"
	EventTypeMobiusACUClosedAccount   MobiusWebhookEventType = "acu.summary.closedaccount"

	// Chargeback events
	EventTypeMobiusChargebackComplete MobiusWebhookEventType = "chargeback.batch.complete"
)

type MobiusBillingError struct {
	Type    string                 `json:"type"`
	Message string                 `json:"message"`
	Context map[string]interface{} `json:"context"`
	Err     error                  `json:"-"`
}

func (be *MobiusBillingError) Error() string {
	if be.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", be.Type, be.Message, be.Err)
	}
	return fmt.Sprintf("%s: %s", be.Type, be.Message)
}

func (be *MobiusBillingError) Unwrap() error {
	return be.Err
}

const (
	ErrorTypeMobiusValidation    = "validation_error"
	ErrorTypeMobiusAmount        = "amount_mismatch"
	ErrorTypeMobiusDuplicate     = "duplicate_transaction"
	ErrorTypeMobiusStatusChange  = "invalid_status_change"
	ErrorTypeMobiusBusinessLogic = "business_logic_error"
	ErrorTypeMobiusDatabase      = "db_error"
	ErrorTypeMobiusNotFound      = "not_found"
)

func newMobiusBillingError(errorType string, message string, context map[string]interface{}, err error) *MobiusBillingError {
	return &MobiusBillingError{
		Type:    errorType,
		Message: message,
		Context: context,
		Err:     err,
	}
}

func (s *MobiusWebhookService) HandleMobiusWebhook(ctx context.Context) error {
	// Use deduplication service if available
	if s.DeduplicationService != nil {
		return s.DeduplicationService.ProcessWebhook(
			ctx,
			s.Data.EventID,
			s.Data.EventType,
			models.ProcessorMobius,
			s.Data,
			s.handleWebhook,
		)
	}

	return s.handleWebhook(ctx)
}

func (s *MobiusWebhookService) handleWebhook(ctx context.Context) error {
	switch s.Data.EventType {
	// Subscription lifecycle events
	case EventTypeMobiusAddSubscription:
		return s.handleAddSubscription(ctx)
	case EventTypeMobiusUpdateSubscription:
		return s.handleUpdateSubscription(ctx)
	case EventTypeMobiusDeleteSubscription:
		return s.handleDeleteSubscription(ctx)

	case EventTypeMobiusChargebackComplete:
		return s.handleChargebackComplete(ctx)

	default:
		// Log unknown event to dead letter queue if service is available
		if s.DeadLetterService != nil {
			dataJSON, err := json.Marshal(s.Data)
			if err == nil {
				s.DeadLetterService.LogUnknownEvent(ctx, "mobius", s.Data.EventType, json.RawMessage(dataJSON), nil, "")
			}
		}
		return fmt.Errorf("unsupported event type: %s", s.Data.EventType)
	}
}

func (s *MobiusWebhookService) handleAddSubscription(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing Mobius subscription add notification")

	mobiusPlanID := s.Data.EventBody.Plan.ID
	mobiusSubID := s.Data.EventBody.SubscriptionID

	if mobiusPlanID == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing plan ID", map[string]interface{}{
			"subscription_id": mobiusSubID,
		}, nil)
	}

	if mobiusSubID == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing subscription ID", map[string]interface{}{
			"plan_id": mobiusPlanID,
		}, nil)
	}

	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationQueueService := NewNotificationQueueService(db)
		subService := NewSubscriptionService(db, priceService, productService, notificationQueueService, nil, s.MobiusClient)

		subscription, err := subService.GetByProcessorSubscriptionID(ctx, ProcessorMobius, mobiusSubID)
		if err != nil {
			return fmt.Errorf("failed to check existing subscription: %w", err)
		}

		price, err := priceService.GetByMobiusPlanID(ctx, mobiusPlanID)
		if err != nil {
			return fmt.Errorf("failed to find price for Mobius plan ID %s: %w", mobiusPlanID, err)
		}

		if subscription.Status != models.StatusPending {
			return fmt.Errorf("invalid subscription state: %w", err)
		}

		if err := subscription.ActivateWithPrice(price); err != nil {
			return fmt.Errorf("failed to activate new subscription: %w", err)
		}

		if err := subscription.Validate(price.Amount); err != nil {
			return fmt.Errorf("failed to validate new subscription: %w", err)
		}

		if err := subService.Update(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"processor_subscription_id": mobiusSubID,
				"plan_id":                   mobiusPlanID,
				"amount":                    price.Amount,
				"billing_cycle_days":        price.BillingCycleDays,
				"period_start":              subscription.CurrentPeriodStartsAt,
				"period_end":                subscription.CurrentPeriodEndsAt,
			}

			subscriptionEventData := SubscriptionEventData{
				EventID:                 uuid.New(),
				Timestamp:               time.Now(),
				ProcessorSubscriptionID: &mobiusSubID,
				Processor:               ProcessorMobius,
				SubscriptionID:          subscription.ID,
				UserID:                  subscription.UserID,
				EventType:               "subscription_created",
				Metadata:                CreateMetadataJSON(metadata),
			}

			if err := s.BillingEventService.LogSubscriptionEvent(ctx, subscriptionEventData); err != nil {
				log.WithError(err).Error("Failed to log subscription creation event to ClickHouse")
			}
		}

		return nil
	})
}

func (s *MobiusWebhookService) handleUpdateSubscription(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing Mobius subscription update notification")

	mobiusPlanID := s.Data.EventBody.Plan.ID
	mobiusSubID := s.Data.EventBody.SubscriptionID

	if mobiusPlanID == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing plan ID", map[string]interface{}{
			"subscription_id": mobiusSubID,
		}, nil)
	}

	if mobiusSubID == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing subscription ID", map[string]interface{}{
			"plan_id": mobiusPlanID,
		}, nil)
	}

	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationQueueService := NewNotificationQueueService(db)
		subService := NewSubscriptionService(db, priceService, productService, notificationQueueService, nil, s.MobiusClient)

		sub, err := subService.GetByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), mobiusSubID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", mobiusSubID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		price, err := priceService.GetByMobiusPlanID(ctx, mobiusPlanID)
		if err != nil {
			return fmt.Errorf("failed to find price for Mobius plan ID %s: %w", mobiusPlanID, err)
		}

		if err := sub.ActivateWithPrice(price); err != nil {
			return fmt.Errorf("failed to activate subscription: %w", err)
		}

		if err := sub.Validate(price.Amount); err != nil {
			return fmt.Errorf("failed to validate subscription: %w", err)
		}

		if err := subService.Update(ctx, sub); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"processor_subscription_id": mobiusSubID,
				"plan_id":                   mobiusPlanID,
				"billing_cycle_days":        price.BillingCycleDays,
				"previous_period_end":       sub.CurrentPeriodEndsAt,
			}

			paymentEventData := PaymentEventData{
				EventID:        uuid.New(),
				SubscriptionID: &sub.ID,
				UserID:         sub.UserID,
				EventType:      "charge_success",
				Processor:      "mobius",
				WebhookSource:  "webhook",
				Metadata:       CreateMetadataJSON(metadata),
			}

			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
				log.WithError(err).Error("Failed to log payment event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID": sub.ID,
			"userID":         sub.UserID,
			"priceID":        price.ID,
		}).Info("Updated subscription successfully")

		return nil
	})
}

func (s *MobiusWebhookService) handleDeleteSubscription(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing Mobius subscription delete notification")

	mobiusSubID := s.Data.EventBody.SubscriptionID

	if mobiusSubID == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing subscription ID", map[string]interface{}{}, nil)
	}

	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := db.NewWithTx(tx)
		priceService := NewPriceService(db)
		productService := NewProductService(db)
		notificationQueueService := NewNotificationQueueService(db)
		subService := NewSubscriptionService(db, priceService, productService, notificationQueueService, nil, s.MobiusClient)

		sub, err := subService.GetByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), mobiusSubID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", mobiusSubID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		cancelType := models.CancelTypeMerchant

		if err := sub.Cancel("Cancelled via Mobius webhook", &cancelType); err != nil {
			return fmt.Errorf("failed to cancel subscription: %w", err)
		}

		if err := subService.Update(ctx, sub); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// Add notification to queue for user and send immediate email
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    sub.UserID,
				EventType: models.NotificationPremiumEnded,
			}
			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver membership ended notification")
			}
		}

		// Log subscription cancellation event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"processor_subscription_id": mobiusSubID,
				"cancel_type":               string(cancelType),
				"cancel_reason":             "Cancelled via Mobius webhook",
				"immediate_cancellation":    false,
			}

			subscriptionEventData := SubscriptionEventData{
				EventID:                 uuid.New(),
				SubscriptionID:          sub.ID,
				UserID:                  sub.UserID,
				EventType:               "subscription_cancelled",
				Processor:               "mobius",
				ProcessorSubscriptionID: &mobiusSubID,
				Metadata:                CreateMetadataJSON(metadata),
				Timestamp:               time.Now(),
			}

			if err := s.BillingEventService.LogSubscriptionEvent(ctx, subscriptionEventData); err != nil {
				log.WithError(err).Error("Failed to log subscription cancellation event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID":          sub.ID,
			"userID":                  sub.UserID,
			"processorSubscriptionID": mobiusSubID,
		}).Info("Cancelled subscription successfully")

		return nil
	})
}

// handleChargebackComplete processes chargeback batch completion notifications
func (s *MobiusWebhookService) handleChargebackComplete(ctx context.Context) error {
	// Chargeback events are typically batch operations containing multiple disputes
	// For now, we'll just log them for administrative purposes

	// Log chargeback batch completion to ClickHouse
	if s.BillingEventService != nil {
		metadata := map[string]interface{}{
			"batch_type":   "chargeback",
			"event_source": "mobius",
			"batch_status": "completed",
		}

		chargebackEventData := ChargebackEventData{
			EventID:   uuid.New(),
			EventType: "batch_processed",
			Processor: "mobius",
			BatchID:   s.Data.EventID, // Use event ID as batch identifier
			Status:    "completed",
			Metadata:  CreateMetadataJSON(metadata),
			Timestamp: time.Now(),
		}

		if err := s.BillingEventService.LogChargebackEvent(ctx, chargebackEventData); err != nil {
			log.WithError(err).Error("Failed to log chargeback batch event to ClickHouse")
		}
	}

	log.WithContext(ctx).WithFields(log.Fields{
		"eventID":   s.Data.EventID,
		"eventType": s.Data.EventType,
	}).Info("Chargeback batch processing completed")

	return nil
}
