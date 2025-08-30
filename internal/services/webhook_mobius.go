package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/doujins-org/doujins-billing/internal/database"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/integrations/mobius"
	"github.com/doujins-org/doujins-billing/internal/services/billing"
	"github.com/doujins-org/doujins-billing/internal/services/notification"
	"github.com/doujins-org/doujins-billing/internal/services/webhook"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

const MobiusProcessorName string = "Mobius"

type MobiusWebhookService struct {
	DB                    *database.DB
	PriceRepo             *repo.PriceRepo
	ProductRepo           *repo.ProductRepo
	Data                  MobiusWebhookEvent
	MobiusClient          *mobius.MobiusClient
	UserRoleGrantRepo     *repo.UserRoleGrantRepo
	DeadLetterService     *webhook.DeadLetterService
	NotificationQueueRepo *repo.NotificationQueueRepo
	NotificationService   *notification.NotificationService
	BillingEventService   *billing.BillingEventService
	DeduplicationService  *webhook.DeduplicationService
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
	ErrorTypeMobiusDatabase      = "database_error"
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

func (s *MobiusWebhookService) logMobiusBillingError(ctx context.Context, billingErr *MobiusBillingError, logFields log.Fields) {
	fields := log.Fields{
		"error_type":    billingErr.Type,
		"error_message": billingErr.Message,
		"error_context": billingErr.Context,
	}

	for k, v := range logFields {
		fields[k] = v
	}

	log.WithContext(ctx).WithFields(fields).Error("Mobius billing error occurred")
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

	// Transaction events
	case EventTypeMobiusTransactionSuccess:
		return s.handleTransactionSuccess(ctx)

	// Automatic Card Updater (ACU) events
	case EventTypeMobiusACUUpdated:
		return s.handleACUUpdated(ctx)
	case EventTypeMobiusACUContactCustomer:
		return s.handleACUContactCustomer(ctx)
	case EventTypeMobiusACUClosedAccount:
		return s.handleACUClosedAccount(ctx)

	// Chargeback events
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

	email := ""
	if s.Data.EventBody.BillingAddress != nil {
		email = s.Data.EventBody.BillingAddress.Email
	}

	processor := models.ProcessorMobius

	if mobiusPlanID == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing plan ID", map[string]interface{}{
			"subscription_id": mobiusSubID,
		}, nil)
	}

	if email == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing email address", map[string]interface{}{
			"plan_id":         mobiusPlanID,
			"subscription_id": mobiusSubID,
		}, nil)
	}

	if mobiusSubID == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing subscription ID", map[string]interface{}{
			"plan_id": mobiusPlanID,
			"email":   email,
		}, nil)
	}

	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := database.NewWithTx(tx)
		userRepo := repo.NewUserRepo(db)
		priceRepo := repo.NewPriceRepo(db)
		subRepo := repo.NewSubscriptionRepo(db)

		user, err := userRepo.GetGoTrueUserByEmail(ctx, email)
		if err != nil {
			return fmt.Errorf("failed to find user with email %s: %w", email, err)
		}

		price, err := priceRepo.GetByMobiusPlanID(ctx, mobiusPlanID)
		if err != nil {
			return fmt.Errorf("failed to find price for Mobius plan ID %s: %w", mobiusPlanID, err)
		}

		subscription, err := subRepo.GetByUserIDAndPriceID(ctx, user.ID, price.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to check existing subscription: %w", err)
		}

		var isNewSubscription bool
		if subscription != nil && subscription.Processor == processor {
			if subscription.ProcessorSubscriptionID == mobiusSubID {
				subscription.ProcessorSubscriptionID = mobiusSubID
			}
		} else {
			isNewSubscription = true
			subscription = &models.Subscription{
				ID:                      uuid.New(),
				UserID:                  user.ID,
				Processor:               processor,
				StartedAt:               time.Now(),
				ProcessorSubscriptionID: mobiusSubID,
			}
		}

		if err := subscription.ActivateWithPrice(price); err != nil {
			return fmt.Errorf("failed to activate new subscription: %w", err)
		}

		if err := subscription.Validate(price.Amount); err != nil {
			return fmt.Errorf("failed to validate new subscription: %w", err)
		}

		if isNewSubscription {
			if err := subRepo.Create(ctx, subscription); err != nil {
				return fmt.Errorf("failed to create subscription: %w", err)
			}
		} else {
			if err := subRepo.Update(ctx, subscription); err != nil {
				return fmt.Errorf("failed to update subscription: %w", err)
			}
		}

		if err := grantRole(
			ctx,
			newGrantRoleParams(user.ID, subscription.ID, processor, price, price.Product, db),
		); err != nil {
			return fmt.Errorf("failed to grant role: %w", err)
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

			subscriptionEventData := billing.SubscriptionEventData{
				EventID:                 uuid.New(),
				UserID:                  user.ID,
				Processor:               "mobius",
				Timestamp:               time.Now(),
				ProcessorSubscriptionID: &mobiusSubID,
				Amount:                  &price.Amount,
				Currency:                price.Currency,
				SubscriptionID:          subscription.ID,
				EventType:               "subscription_created",
				Metadata:                billing.CreateMetadataJSON(metadata),
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
		db := database.NewWithTx(tx)
		subRepo := repo.NewSubscriptionRepo(db)
		priceRepo := repo.NewPriceRepo(db)

		sub, err := subRepo.GetByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), mobiusSubID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", mobiusSubID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		price, err := priceRepo.GetByMobiusPlanID(ctx, mobiusPlanID)
		if err != nil {
			return fmt.Errorf("failed to find price for Mobius plan ID %s: %w", mobiusPlanID, err)
		}

		if err := sub.ActivateWithPrice(price); err != nil {
			return fmt.Errorf("failed to activate subscription: %w", err)
		}

		if err := sub.Validate(price.Amount); err != nil {
			return fmt.Errorf("failed to validate subscription: %w", err)
		}

		if err := subRepo.Update(ctx, sub); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// grantParams := newGrantRoleParams(sub.UserID, sub.ID, models.ProcessorMobius, price, price.Product, db)
		// if err := grantRole(ctx, grantParams); err != nil {
		// 	return fmt.Errorf("failed to grant role: %w", err)
		// }
		// Role granting will be handled by transaction.sale.success webhook
		// TO DO: verify this behavior

		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"processor_subscription_id": mobiusSubID,
				"plan_id":                   mobiusPlanID,
				"billing_cycle_days":        price.BillingCycleDays,
				"previous_period_end":       sub.CurrentPeriodEndsAt,
			}

			paymentEventData := billing.PaymentEventData{
				EventID:        uuid.New(),
				SubscriptionID: &sub.ID,
				UserID:         sub.UserID,
				EventType:      "charge_success",
				Processor:      "mobius",
				Amount:         &price.Amount,
				Currency:       price.Currency,
				WebhookSource:  "webhook",
				Metadata:       billing.CreateMetadataJSON(metadata),
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
		db := database.NewWithTx(tx)
		subRepo := repo.NewSubscriptionRepo(db)

		sub, err := subRepo.GetByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), mobiusSubID)
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

		if err := subRepo.Update(ctx, sub); err != nil {
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

			subscriptionEventData := billing.SubscriptionEventData{
				EventID:                 uuid.New(),
				SubscriptionID:          sub.ID,
				UserID:                  sub.UserID,
				EventType:               "subscription_cancelled",
				Processor:               "mobius",
				ProcessorSubscriptionID: &mobiusSubID,
				Metadata:                billing.CreateMetadataJSON(metadata),
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

// handleTransactionSuccess processes successful transaction payments
func (s *MobiusWebhookService) handleTransactionSuccess(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing Mobius transaction success notification")

	email := ""
	if s.Data.EventBody.BillingAddress != nil {
		email = s.Data.EventBody.BillingAddress.Email
	}
	transactionID := s.Data.EventBody.ProcessorID
	planID := s.Data.EventBody.Plan.ID
	amountStr := s.Data.EventBody.Plan.Amount

	if email == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing email address", map[string]interface{}{
			"transaction_id": transactionID,
			"plan_id":        planID,
		}, nil)
	}

	if transactionID == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing transaction ID", map[string]interface{}{
			"email":   email,
			"plan_id": planID,
		}, nil)
	}

	if planID == "" {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing plan ID", map[string]interface{}{
			"email":          email,
			"transaction_id": transactionID,
		}, nil)
	}

	// Parse amount
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Failed to parse transaction amount", map[string]interface{}{
			"amount_string":  amountStr,
			"transaction_id": transactionID,
			"plan_id":        planID,
		}, err)
	}

	if amount <= 0 {
		return newMobiusBillingError(ErrorTypeMobiusValidation, "Invalid transaction amount", map[string]interface{}{
			"amount":         amount,
			"transaction_id": transactionID,
			"plan_id":        planID,
		}, nil)
	}

	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := database.NewWithTx(tx)
		userRepo := repo.NewUserRepo(db)
		priceRepo := repo.NewPriceRepo(db)
		purchaseRepo := repo.NewPurchaseRepo(db)

		// 1. Check for duplicate transaction ID
		existingPurchase, err := purchaseRepo.GetByTransactionID(ctx, models.ProcessorMobius, transactionID)
		if err == nil && existingPurchase != nil {
			log.WithContext(ctx).WithFields(log.Fields{
				"transactionID": transactionID,
				"existingID":    existingPurchase.ID,
			}).Info("Duplicate transaction detected, skipping processing")
			return nil // Idempotency - already processed
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to check for duplicate transaction: %w", err)
		}

		// 2. Find user by email
		user, err := userRepo.GetGoTrueUserByEmail(ctx, email)
		if err != nil {
			return fmt.Errorf("failed to find user with email %s: %w", email, err)
		}

		// 3. Find price by Mobius plan ID
		price, err := priceRepo.GetByMobiusPlanID(ctx, planID)
		if err != nil {
			return fmt.Errorf("failed to find price for Mobius plan ID %s: %w", planID, err)
		}

		// 4. Validate transaction amount matches expected price (with 2% tolerance like CCBill)
		expectedAmount := price.Amount
		tolerance := expectedAmount * 0.02
		if amount < (expectedAmount-tolerance) || amount > (expectedAmount+tolerance) {
			billingErr := newMobiusBillingError(ErrorTypeMobiusAmount,
				"Transaction amount does not match expected price",
				map[string]interface{}{
					"expected_amount": expectedAmount,
					"billed_amount":   amount,
					"tolerance":       tolerance,
					"price_id":        price.ID.String(),
					"plan_id":         planID,
				}, nil)

			s.logMobiusBillingError(ctx, billingErr, log.Fields{
				"transaction_id": transactionID,
				"email":          email,
			})
			return billingErr
		}

		// 5. Get product to determine role configuration
		productRepo := repo.NewProductRepo(db)
		product, err := productRepo.GetByID(ctx, price.ProductID)
		if err != nil {
			return fmt.Errorf("failed to get product: %w", err)
		}

		var userRoleGrant *models.UserRoleGrant
		var extensionDays int

		// 5. Grant/extend role if product has role configured
		if product.RoleID != nil {
			userRoleGrantRepo := repo.NewUserRoleGrantRepo(db)

			// Determine extension days from product
			if product.RoleDurationDays != nil && *product.RoleDurationDays > 0 {
				extensionDays = *product.RoleDurationDays
			} else if price.BillingCycleDays != nil && *price.BillingCycleDays > 0 {
				extensionDays = *price.BillingCycleDays
			} else {
				extensionDays = 30 // Default fallback
			}

			// Extend the user's role expiration
			grant, _, err := userRoleGrantRepo.ExtendRoleExpiration(ctx, user.ID, *product.RoleID, extensionDays)
			if err != nil {
				return fmt.Errorf("failed to extend role expiration: %w", err)
			}
			userRoleGrant = grant
		}

		// 6. Create Purchase record
		purchase := &models.Purchase{
			ID:              uuid.New(),
			UserID:          user.ID,
			PriceID:         price.ID,
			UserRoleGrantID: nil, // Set if role was granted
			Processor:       models.ProcessorMobius,
			TransactionID:   transactionID,
			Amount:          amount,
			Currency:        price.Currency,
			ExtensionDays:   nil, // Set if role was extended
			PurchasedAt:     time.Now(),
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		// Set optional fields if role was granted
		if userRoleGrant != nil {
			purchase.UserRoleGrantID = &userRoleGrant.ID
			purchase.ExtensionDays = &extensionDays
		}

		if err := purchaseRepo.Create(ctx, purchase); err != nil {
			return fmt.Errorf("failed to create purchase record: %w", err)
		}

		// 7. Log transaction success event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"transaction_id": transactionID,
				"plan_id":        planID,
				"product_id":     product.ID.String(),
				"role_granted":   product.RoleID != nil,
				"extension_days": extensionDays,
				"amount":         amount,
				"email":          email,
			}

			transactionEventData := billing.TransactionEventData{
				EventID:        uuid.New(),
				UserID:         &user.ID,
				SubscriptionID: nil, // Will be set if subscription-based
				EventType:      "payment_succeeded",
				Processor:      "mobius",
				TransactionID:  transactionID,
				Amount:         &amount,
				Currency:       price.Currency,
				Status:         "completed",
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now(),
			}

			if err := s.BillingEventService.LogTransactionEvent(ctx, transactionEventData); err != nil {
				log.WithError(err).Error("Failed to log transaction success event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"userID":        user.ID,
			"transactionID": transactionID,
			"planID":        planID,
			"productID":     product.ID,
			"roleGranted":   product.RoleID != nil,
			"extensionDays": extensionDays,
			"purchaseID":    purchase.ID,
		}).Info("Successfully processed transaction success webhook")

		return nil
	})
}

// handleACUUpdated processes automatic card update notifications
func (s *MobiusWebhookService) handleACUUpdated(ctx context.Context) error {
	// Extract customer/subscription info from webhook
	subscriptionID := s.Data.EventBody.SubscriptionID
	email := s.Data.EventBody.BillingAddress.Email
	vaultID := s.Data.EventBody.VaultID

	// Extract updated card details from webhook (these would come from actual Mobius webhook)
	var newLastFour, newCardType, newExpiryDate *string
	if cardInfo := s.Data.EventBody.PaymentMethod; cardInfo != nil {
		if cardInfo.LastFour != "" {
			newLastFour = &cardInfo.LastFour
		}
		if cardInfo.CardType != "" {
			newCardType = &cardInfo.CardType
		}
		if cardInfo.ExpiryDate != "" {
			newExpiryDate = &cardInfo.ExpiryDate
		}
	}

	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := database.NewWithTx(tx)
		paymentMethodRepo := repo.NewPaymentMethodRepo(db)
		userRepo := repo.NewUserRepo(db)

		// Find user by email
		user, err := userRepo.GetGoTrueUserByEmail(ctx, email)
		if err != nil {
			log.WithError(err).Warn("Could not find user for ACU update webhook")
			return nil // Don't fail the webhook for missing user
		}

		// Find payment method by vault ID and processor
		paymentMethod, err := paymentMethodRepo.GetByVaultID(ctx, models.ProcessorMobius, vaultID)
		if err != nil {
			log.WithError(err).Warn("Could not find payment method for ACU update webhook")
			return nil // Don't fail the webhook for missing payment method
		}

		// Verify payment method belongs to user
		if paymentMethod.UserID != user.ID {
			log.WithFields(log.Fields{
				"vaultID":           vaultID,
				"paymentMethodUser": paymentMethod.UserID,
				"webhookUser":       user.ID,
			}).Error("Payment method user mismatch in ACU update webhook")
			return nil
		}

		// Update payment method - ACU methods were removed since we don't track ACU status
		// Just mark as active since auto-update was successful
		paymentMethod.IsActive = true
		paymentMethod.FailureReason = nil

		if err := paymentMethodRepo.Update(ctx, paymentMethod); err != nil {
			return fmt.Errorf("failed to update payment method after ACU update: %w", err)
		}

		// Log ACU update event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"subscription_id":      subscriptionID,
				"vault_id":             vaultID,
				"email":                email,
				"acu_status":           "automatically_updated",
				"card_updated":         true,
				"payment_method_id":    paymentMethod.ID.String(),
				"card_details_updated": newLastFour != nil || newCardType != nil || newExpiryDate != nil,
			}

			// Convert string subscription ID to UUID pointer
			var subID *uuid.UUID
			if subscriptionID != "" {
				if parsedID, err := uuid.Parse(subscriptionID); err == nil {
					subID = &parsedID
				}
			}

			acuEventData := billing.ACUEventData{
				EventID:        uuid.New(),
				SubscriptionID: subID,
				EventType:      "card_automatically_updated",
				Processor:      "mobius",
				UpdateStatus:   "success",
				RequiresAction: false,
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now(),
			}

			if err := s.BillingEventService.LogACUEvent(ctx, acuEventData); err != nil {
				log.WithError(err).Error("Failed to log ACU update event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"userID":             user.ID,
			"subscriptionID":     subscriptionID,
			"vaultID":            vaultID,
			"paymentMethodID":    paymentMethod.ID,
			"cardDetailsUpdated": newLastFour != nil || newCardType != nil || newExpiryDate != nil,
		}).Info("Payment method automatically updated via ACU")

		return nil
	})
}

// handleACUContactCustomer processes ACU notifications requiring customer action
func (s *MobiusWebhookService) handleACUContactCustomer(ctx context.Context) error {
	subscriptionID := s.Data.EventBody.SubscriptionID
	email := s.Data.EventBody.BillingAddress.Email

	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := database.NewWithTx(tx)
		userRepo := repo.NewUserRepo(db)
		subRepo := repo.NewSubscriptionRepo(db)

		// Find user and subscription
		user, err := userRepo.GetGoTrueUserByEmail(ctx, email)
		if err != nil {
			log.WithError(err).Warn("Could not find user for ACU contact customer webhook")
			return nil // Don't fail the webhook for missing user
		}

		sub, err := subRepo.GetByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), subscriptionID)
		if err != nil {
			log.WithError(err).Warn("Could not find subscription for ACU contact customer webhook")
			return nil // Don't fail the webhook for missing subscription
		}

		// Add notification to queue for user to update payment method
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    user.ID,
				EventType: models.NotificationPaymentMethodUpdateRequired,
				Data: map[string]interface{}{
					"subscription_id": sub.ID.String(),
					"reason":          "Card update required by payment processor",
				},
			}

			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver payment method update notification")
			}
		}

		// Log ACU contact customer event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"subscription_id":   subscriptionID,
				"email":             email,
				"acu_status":        "contact_customer",
				"requires_action":   true,
				"notification_sent": true,
			}

			// Convert string subscription ID to UUID pointer
			var subID *uuid.UUID
			if subscriptionID != "" {
				if parsedID, err := uuid.Parse(subscriptionID); err == nil {
					subID = &parsedID
				}
			}

			acuEventData := billing.ACUEventData{
				EventID:        uuid.New(),
				SubscriptionID: subID,
				EventType:      "card_update_required",
				Processor:      "mobius",
				UpdateStatus:   "pending",
				RequiresAction: true,
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now(),
			}

			if err := s.BillingEventService.LogACUEvent(ctx, acuEventData); err != nil {
				log.WithError(err).Error("Failed to log ACU contact customer event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"userID":         user.ID,
			"subscriptionID": sub.ID,
			"email":          email,
		}).Info("Customer contact required for card update")

		return nil
	})
}

// handleACUClosedAccount processes notifications when customer's payment account is closed
func (s *MobiusWebhookService) handleACUClosedAccount(ctx context.Context) error {
	subscriptionID := s.Data.EventBody.SubscriptionID
	email := s.Data.EventBody.BillingAddress.Email
	vaultID := s.Data.EventBody.VaultID

	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := database.NewWithTx(tx)
		userRepo := repo.NewUserRepo(db)
		subRepo := repo.NewSubscriptionRepo(db)
		paymentMethodRepo := repo.NewPaymentMethodRepo(db)

		// Find user and subscription
		user, err := userRepo.GetGoTrueUserByEmail(ctx, email)
		if err != nil {
			log.WithError(err).Warn("Could not find user for ACU closed account webhook")
			return nil // Don't fail the webhook for missing user
		}

		sub, err := subRepo.GetByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), subscriptionID)
		if err != nil {
			log.WithError(err).Warn("Could not find subscription for ACU closed account webhook")
			return nil // Don't fail the webhook for missing subscription
		}

		// Find and mark payment method as inactive
		if vaultID != "" {
			paymentMethod, err := paymentMethodRepo.GetByVaultID(ctx, models.ProcessorMobius, vaultID)
			if err != nil {
				log.WithError(err).Warn("Could not find payment method for ACU closed account webhook")
			} else if paymentMethod.UserID == user.ID {
				// Mark payment method as inactive due to closed account
				paymentMethod.MarkInactive("Payment account closed by bank")

				if err := paymentMethodRepo.Update(ctx, paymentMethod); err != nil {
					log.WithError(err).Error("Failed to mark payment method as inactive after account closure")
				} else {
					log.WithContext(ctx).WithFields(log.Fields{
						"paymentMethodID": paymentMethod.ID,
						"vaultID":         vaultID,
					}).Info("Payment method marked inactive due to account closure")
				}
			}
		}

		// Put subscription in grace period or require new payment method
		now := time.Now()
		sub.Status = models.StatusPastDue
		sub.UpdatedAt = now

		if err := subRepo.Update(ctx, sub); err != nil {
			return fmt.Errorf("failed to update subscription status: %w", err)
		}

		// Add urgent notification to user about closed account and send immediate email
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    user.ID,
				EventType: models.NotificationPaymentMethodFailed,
				Data: map[string]interface{}{
					"subscription_id": sub.ID.String(),
					"reason":          "Payment account closed by bank",
					"urgency":         "high",
					"vault_id":        vaultID,
				},
			}

			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver closed account notification")
			}
		}

		// Log ACU closed account event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"subscription_id":            subscriptionID,
				"vault_id":                   vaultID,
				"email":                      email,
				"acu_status":                 "closed_account",
				"account_closed":             true,
				"subscription_status":        string(sub.Status),
				"payment_method_deactivated": vaultID != "",
			}

			// Convert string subscription ID to UUID pointer
			var subID *uuid.UUID
			if subscriptionID != "" {
				if parsedID, err := uuid.Parse(subscriptionID); err == nil {
					subID = &parsedID
				}
			}

			acuEventData := billing.ACUEventData{
				EventID:        uuid.New(),
				SubscriptionID: subID,
				EventType:      "account_closed",
				Processor:      "mobius",
				UpdateStatus:   "failed",
				RequiresAction: true,
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now(),
			}

			if err := s.BillingEventService.LogACUEvent(ctx, acuEventData); err != nil {
				log.WithError(err).Error("Failed to log ACU closed account event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"userID":         user.ID,
			"subscriptionID": sub.ID,
			"vaultID":        vaultID,
			"email":          email,
		}).Info("Payment account closed, subscription put on hold and payment method deactivated")

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

		chargebackEventData := billing.ChargebackEventData{
			EventID:   uuid.New(),
			EventType: "batch_processed",
			Processor: "mobius",
			BatchID:   s.Data.EventID, // Use event ID as batch identifier
			Status:    "completed",
			Metadata:  billing.CreateMetadataJSON(metadata),
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
