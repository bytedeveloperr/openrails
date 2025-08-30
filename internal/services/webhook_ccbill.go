package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/services/billing"
	"github.com/doujins-org/doujins-billing/internal/services/notification"
	"github.com/doujins-org/doujins-billing/internal/services/webhook"
	"github.com/google/uuid"
	"github.com/supabase-community/gotrue-go/types"

	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

type CCBillWebhookService struct {
	Data                CCBillWebhookEvent
	DB                  *db.DB
	CCBillClient        *ccbill.RESTClient
	NotificationService *notification.NotificationService
	DeadLetterService   *webhook.DeadLetterService
	BillingEventService *billing.BillingEventService
}

// Repository methods for CCBillWebhookService

// User repository methods
func (s *CCBillWebhookService) GetGoTrueUserByEmail(ctx context.Context, email string) (*types.User, error) {
	// This would typically call the GoTrue API or database
	// For now, return a placeholder implementation
	return nil, fmt.Errorf("user not found: %s", email)
}

func (s *CCBillWebhookService) GetGoTrueUserByID(ctx context.Context, userID uuid.UUID) (*types.User, error) {
	// This would typically call the GoTrue API or database
	// For now, return a placeholder implementation
	return nil, fmt.Errorf("user not found: %s", userID)
}

// Price repository methods
func (s *CCBillWebhookService) GetPriceByCCBillPriceID(ctx context.Context, ccbillPriceID string) (*models.Price, error) {
	var price models.Price
	err := s.DB.GetDB().NewSelect().
		Model(&price).
		Where("ccbill_price_id = ?", ccbillPriceID).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get price by CCBill price ID: %w", err)
	}
	return &price, nil
}

func (s *CCBillWebhookService) GetPriceByID(ctx context.Context, id uuid.UUID) (*models.Price, error) {
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

// Subscription repository methods
func (s *CCBillWebhookService) GetSubscriptionByUserIDAndPriceID(ctx context.Context, userID, priceID uuid.UUID) (*models.Subscription, error) {
	var subscription models.Subscription
	err := s.DB.GetDB().NewSelect().
		Model(&subscription).
		Where("user_id = ?", userID).
		Where("price_id = ?", priceID).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription by user ID and price ID: %w", err)
	}
	return &subscription, nil
}

func (s *CCBillWebhookService) GetSubscriptionByProcessorSubscriptionID(ctx context.Context, processor, processorSubID string) (*models.Subscription, error) {
	var subscription models.Subscription
	err := s.DB.GetDB().NewSelect().
		Model(&subscription).
		Where("processor = ?", processor).
		Where("processor_subscription_id = ?", processorSubID).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription by processor subscription ID: %w", err)
	}
	return &subscription, nil
}

func (s *CCBillWebhookService) CreateSubscription(ctx context.Context, subscription *models.Subscription) error {
	_, err := s.DB.GetDB().NewInsert().
		Model(subscription).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}
	return nil
}

func (s *CCBillWebhookService) UpdateSubscription(ctx context.Context, subscription *models.Subscription) error {
	_, err := s.DB.GetDB().NewUpdate().
		Model(subscription).
		Where("id = ?", subscription.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}
	return nil
}

// UserRoleGrant repository methods
func (s *CCBillWebhookService) RevokeUserRolesBySubSourceID(ctx context.Context, subID uuid.UUID) error {
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

// Purchase repository methods
func (s *CCBillWebhookService) CreatePurchase(ctx context.Context, purchase *models.Purchase) error {
	_, err := s.DB.GetDB().NewInsert().
		Model(purchase).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create purchase: %w", err)
	}
	return nil
}

func (s *CCBillWebhookService) UpdatePurchase(ctx context.Context, purchase *models.Purchase) error {
	_, err := s.DB.GetDB().NewUpdate().
		Model(purchase).
		Where("id = ?", purchase.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update purchase: %w", err)
	}
	return nil
}

// ExtendRoleExpiration method for RoleGrantService interface
func (s *CCBillWebhookService) ExtendRoleExpiration(ctx context.Context, userID, roleID uuid.UUID, days int) (*models.UserRoleGrant, time.Time, error) {
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

type CCBillWebhookEventType = string

const CCBillProcessorType models.ProcessorType = "ccbill"

const (
	EventTypeNewSaleSuccess     CCBillWebhookEventType = "NewSaleSuccess"
	EventTypeNewSaleFailure     CCBillWebhookEventType = "NewSaleFailure"
	EventTypeRenewalSuccess     CCBillWebhookEventType = "RenewalSuccess"
	EventTypeRenewalFailure     CCBillWebhookEventType = "RenewalFailure"
	EventTypeUpgradeSuccess     CCBillWebhookEventType = "UpgradeSuccess"
	EventTypeUpgradeFailure     CCBillWebhookEventType = "UpgradeFailure"
	EventTypeCancellation       CCBillWebhookEventType = "Cancellation"
	EventTypeExpiration         CCBillWebhookEventType = "Expiration"
	EventTypeBillingDateChange  CCBillWebhookEventType = "BillingDateChange"
	EventTypeCustomerDataUpdate CCBillWebhookEventType = "CustomerDataUpdate"
	EventTypeUserReactivation   CCBillWebhookEventType = "UserReactivation"
	EventTypeRefund             CCBillWebhookEventType = "Refund"
	EventTypeVoid               CCBillWebhookEventType = "Void"
	EventTypeChargeback         CCBillWebhookEventType = "Chargeback"
)

type BillingError struct {
	Type    string                 `json:"type"`
	Message string                 `json:"message"`
	Context map[string]interface{} `json:"context"`
	Err     error                  `json:"-"`
}

func (be *BillingError) Error() string {
	if be.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", be.Type, be.Message, be.Err)
	}
	return fmt.Sprintf("%s: %s", be.Type, be.Message)
}

func (be *BillingError) Unwrap() error {
	return be.Err
}

const (
	ErrorTypeValidation    = "validation_error"
	ErrorTypeAmount        = "amount_mismatch"
	ErrorTypeDuplicate     = "duplicate_transaction"
	ErrorTypeStatusChange  = "invalid_status_change"
	ErrorTypeBusinessLogic = "business_logic_error"
	ErrorTypeDatabase      = "database_error"
	ErrorTypeNotFound      = "not_found"
)

func (s *CCBillWebhookService) HandleCCBillWebhook(ctx context.Context) error {
	switch s.Data.EventType {
	case EventTypeNewSaleSuccess:
		return s.handleNewSaleSuccess(ctx)
	case EventTypeNewSaleFailure:
		return s.handleNewSaleFailure(ctx)
	case EventTypeRenewalSuccess:
		return s.handleRenewalSuccess(ctx)
	case EventTypeRenewalFailure:
		return s.handleRenewalFailure(ctx)
	case EventTypeUpgradeSuccess:
		return s.handleUpgradeSuccess(ctx)
	case EventTypeUpgradeFailure:
		return s.handleUpgradeFailure(ctx)
	case EventTypeCancellation:
		return s.handleCancel(ctx)
	case EventTypeExpiration:
		return s.handleExpiration(ctx)
	case EventTypeBillingDateChange:
		return s.handleBillingDateChange(ctx)
	case EventTypeCustomerDataUpdate:
		return s.handleCustomerDataUpdate(ctx)
	case EventTypeUserReactivation:
		return s.handleUserReactivation(ctx)
	case EventTypeRefund:
		return s.handleRefund(ctx)
	case EventTypeVoid:
		return s.handleVoid(ctx)
	case EventTypeChargeback:
		return s.handleChargeback(ctx)
	default:
		// Log unknown event to dead letter queue if service is available
		if s.DeadLetterService != nil {
			s.DeadLetterService.LogUnknownEvent(ctx, "ccbill", s.Data.EventType, json.RawMessage(s.Data.EventBody), nil, "")
		}
		return fmt.Errorf("unsupported event type: %s", s.Data.EventType)
	}
}

func (s *CCBillWebhookService) handleNewSaleSuccess(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing CCBill webhook notification")

	var data CCBillNewSaleSuccessEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	email := data.Email
	formID := data.FlexID
	formName := data.FormName
	ccBillSubID := data.SubscriptionID
	transactionID := data.TransactionID
	billedAmountStr := data.BilledInitialPrice

	billedAmount, err := strconv.ParseFloat(billedAmountStr, 64)
	if err != nil {
		return fmt.Errorf("failed to parse billedInitialPrice '%s': %w", data.BilledInitialPrice, err)
	}

	if billedAmount <= 0 {
		return fmt.Errorf("invalid billedAmount: %f - must be greater than 0", billedAmount)
	}

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		processor := models.ProcessorCCBill

								
		user, err := s.GetGoTrueUserByEmail(ctx, email)
		if err != nil {
			return fmt.Errorf("failed to find user with email %s: %w", email, err)
		}

		cfg := s.CCBillClient.Config()
		if formID != cfg.FormID {
			return fmt.Errorf("payment form id mismatch: got %s, want %s", formID, cfg.FormID)
		}

		if formName != cfg.FormName {
			return fmt.Errorf("payment form name mismatch: got %s, want %s", formName, cfg.FormName)
		}

		price, err := s.GetPriceByCCBillPriceID(ctx, data.FlexID)
		if err != nil {
			return fmt.Errorf("failed to find price for CCBill price ID %s: %w", data.FlexID, err)
		}

		expectedAmount := price.Amount
		tolerance := expectedAmount * 0.02
		if billedAmount < (expectedAmount-tolerance) || billedAmount > (expectedAmount+tolerance) {
			billingErr := newBillingError(ErrorTypeAmount,
				"Billed amount does not match expected price",
				map[string]interface{}{
					"expected_amount": expectedAmount,
					"billed_amount":   billedAmount,
					"tolerance":       tolerance,
					"price_id":        price.ID.String(),
					"ccbill_price_id": price.CCBillPriceID,
				}, nil)

			s.logBillingError(ctx, billingErr, log.Fields{
				"transaction_id": transactionID,
				"email":          email,
			})
			return billingErr
		}

		subscription, err := s.GetSubscriptionByUserIDAndPriceID(ctx, user.ID, price.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		// If user already has a subscription with the same price,
		// update the existing subscription only if the processor match, otherwise create a new subscription.
		var isNewSubscription bool
		if subscription != nil && subscription.Processor == processor {
			if subscription.ProcessorSubscriptionID != ccBillSubID {
				subscription.ProcessorSubscriptionID = ccBillSubID
			}
		} else {
			isNewSubscription = true

			subscription = &models.Subscription{
				ID:                      uuid.New(),
				UserID:                  user.ID,
				Processor:               processor,
				StartedAt:               time.Now(),
				ProcessorSubscriptionID: ccBillSubID,
			}
		}

		if err := subscription.ActivateWithPrice(price); err != nil {
			return err
		}

		if err := subscription.Validate(billedAmount); err != nil {
			return err
		}

		if isNewSubscription {
			if err := s.CreateSubscription(ctx, subscription); err != nil {
				return fmt.Errorf("failed to create subscription: %w", err)
			}
		} else {
			if err := s.UpdateSubscription(ctx, subscription); err != nil {
				return err
			}
		}

		if err := grantRole(
			ctx,
			newGrantRoleParams(user.ID, subscription.ID, processor, price, price.Product, s)); err != nil {
			return fmt.Errorf("failed to grant role: %w", err)
		}

		// Log payment event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"transaction_id": transactionID,
				"processor":      "ccbill",
				"event_source":   "webhook",
				"amount":         billedAmount,
			}

			paymentEventData := billing.PaymentEventData{
				EventID:        uuid.New(),
				SubscriptionID: &subscription.ID,
				UserID:         subscription.UserID,
				EventType:      "charge_success",
				Processor:      "ccbill",
				Amount:         &billedAmount,
				Currency:       "USD",
				WebhookSource:  "webhook",
				BillingInfo:    billing.CreateMetadataJSON(map[string]interface{}{}), // No billing info from webhook
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now().UTC(),
			}

			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
				log.WithError(err).Error("Failed to log payment event to ClickHouse")
			}
		}

		// Add notification to queue for user and send immediate email
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    user.ID,
				EventType: models.NotificationPremiumStarted,
			}
			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver membership started notification")
			}
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleNewSaleFailure(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing CCBill new sale failure notification")

	var data CCBillNewSaleFailureEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	transactionID := data.TransactionID
	email := data.Email
	formID := data.FlexID
	formName := data.FormName
	failureCode := data.FailureCode
	failureReason := data.FailureReason

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
				
		// Find user by email
		user, err := s.GetGoTrueUserByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.WithContext(ctx).WithFields(log.Fields{
					"email":          email,
					"failure_code":   failureCode,
					"failure_reason": failureReason,
					"transaction_id": transactionID,
				}).Warn("New sale failure for unknown user - no notification sent")
				return nil // Don't fail webhook processing for unknown users
			}
			return fmt.Errorf("failed to find user with email %s: %w", email, err)
		}

		// Validate form configuration
		cfg := s.CCBillClient.Config()
		if formID != cfg.FormID {
			log.WithContext(ctx).WithFields(log.Fields{
				"received_form_id": formID,
				"expected_form_id": cfg.FormID,
				"email":            email,
			}).Warn("Payment form ID mismatch in new sale failure")
		}
		if formName != cfg.FormName {
			log.WithContext(ctx).WithFields(log.Fields{
				"received_form_name": formName,
				"expected_form_name": cfg.FormName,
				"email":              email,
			}).Warn("Payment form name mismatch in new sale failure")
		}

		// Log payment failure event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"transaction_id": transactionID,
				"processor":      "ccbill",
				"event_source":   "webhook",
				"failure_code":   failureCode,
				"failure_reason": failureReason,
				"form_id":        formID,
				"form_name":      formName,
			}

			paymentEventData := billing.PaymentEventData{
				EventID:       uuid.New(),
				UserID:        user.ID,
				EventType:     "charge_failed",
				Processor:     "ccbill",
				Currency:      "USD",
				BillingInfo:   billing.CreateMetadataJSON(map[string]interface{}{"initial_signup": true}),
				WebhookSource: "webhook",
				Metadata:      billing.CreateMetadataJSON(metadata),
				Timestamp:     time.Now().UTC(),
			}

			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
				log.WithError(err).Error("Failed to log new sale failure event to ClickHouse")
			}
		}

		// Add notification to queue for user about payment failure and send immediate email
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    user.ID,
				EventType: models.NotificationPaymentMethodFailed,
			}
			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver new sale failure notification")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"userID":        user.ID,
			"email":         email,
			"failureCode":   failureCode,
			"failureReason": failureReason,
			"transactionID": transactionID,
		}).Info("Handled new sale failure")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleUpgradeSuccess(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing CCBill upgrade success notification")

	var data CCBillUpgradeSuccessEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	flexID := data.FlexID
	formName := data.FormName
	billedAmount := data.Amount
	ccBillSubID := data.SubscriptionID
	transactionID := data.TransactionID
	originalSubscriptionID := data.OriginalSubscriptionID

	if billedAmount <= 0 {
		return fmt.Errorf("invalid billedAmount: %f - must be greater than 0", billedAmount)
	}

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
						
		// Find subscription by processor subscription ID
		subscription, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), originalSubscriptionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", originalSubscriptionID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		newPrice, err := s.GetPriceByCCBillPriceID(ctx, data.FlexID)
		if err != nil {
			return fmt.Errorf("failed to find new price for CCBill price ID %s: %w", data.FlexID, err)
		}

		// Validate the billed amount matches the new price
		expectedAmount := newPrice.Amount
		tolerance := expectedAmount * 0.02
		if billedAmount < (expectedAmount-tolerance) || billedAmount > (expectedAmount+tolerance) {
			billingErr := newBillingError(ErrorTypeAmount,
				"Upgrade billed amount does not match expected price",
				map[string]interface{}{
					"expected_amount":          expectedAmount,
					"billed_amount":            billedAmount,
					"tolerance":                tolerance,
					"new_price_id":             newPrice.ID.String(),
					"new_flex_id":              flexID,
					"original_subscription_id": originalSubscriptionID,
				}, nil)

			s.logBillingError(ctx, billingErr, log.Fields{
				"transaction_id":  transactionID,
				"subscription_id": subscription.ID,
			})
			return billingErr
		}

		if err := subscription.ActivateWithPrice(newPrice); err != nil {
			return fmt.Errorf("failed to activate subscription: %w", err)
		}

		subscription.ProcessorSubscriptionID = ccBillSubID

		if err := subscription.Validate(billedAmount); err != nil {
			return fmt.Errorf("failed to validate subscription: %w", err)
		}

		if err := s.UpdateSubscription(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		// Grant role for new subscription tier
		grantParams := newGrantRoleParams(subscription.UserID, subscription.ID, models.ProcessorCCBill, newPrice, newPrice.Product, s)
		if err := grantRole(ctx, grantParams); err != nil {
			return fmt.Errorf("failed to grant role for upgraded subscription: %w", err)
		}

		// Log upgrade payment event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"transaction_id":           transactionID,
				"processor":                "ccbill",
				"event_source":             "webhook",
				"event_type":               "upgrade",
				"amount":                   billedAmount,
				"new_flex_id":              data.FlexID,
				"new_form_name":            formName,
				"original_subscription_id": originalSubscriptionID,
				"previous_price_id":        subscription.PriceID.String(),
				"new_price_id":             newPrice.ID.String(),
			}

			paymentEventData := billing.PaymentEventData{
				EventID:        uuid.New(),
				SubscriptionID: &subscription.ID,
				UserID:         subscription.UserID,
				EventType:      "charge_success",
				Processor:      "ccbill",
				Amount:         &billedAmount,
				Currency:       "USD",
				BillingInfo:    billing.CreateMetadataJSON(map[string]interface{}{"upgrade": true}),
				WebhookSource:  "webhook",
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now().UTC(),
			}

			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
				log.WithError(err).Error("Failed to log upgrade payment event to ClickHouse")
			}
		}

		// Add notification to queue for user about successful upgrade and send immediate email
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    subscription.UserID,
				EventType: models.NotificationPremiumRenewed, // Use renewed for upgrades
			}
			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver upgrade success notification")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID":         subscription.ID,
			"userID":                 subscription.UserID,
			"newPriceID":             newPrice.ID,
			"billedAmount":           billedAmount,
			"transactionID":          transactionID,
			"newFlexID":              flexID,
			"originalSubscriptionID": originalSubscriptionID,
		}).Info("Processed subscription upgrade successfully")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleUpgradeFailure(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing CCBill upgrade failure notification")

	var data CCBillUpgradeFailureEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	transactionID := data.TransactionID
	email := data.Email
	failureCode := data.FailureCode
	failureReason := data.FailureReason
	originalSubscriptionID := data.OriginalSubscriptionID

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
				
		// Find user by email
		user, err := s.GetGoTrueUserByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.WithContext(ctx).WithFields(log.Fields{
					"email":                    email,
					"failure_code":             failureCode,
					"failure_reason":           failureReason,
					"transaction_id":           transactionID,
					"original_subscription_id": originalSubscriptionID,
				}).Warn("Upgrade failure for unknown user - no notification sent")
				return nil // Don't fail webhook processing for unknown users
			}
			return fmt.Errorf("failed to find user with email %s: %w", email, err)
		}

		// Log upgrade failure event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"transaction_id":           transactionID,
				"processor":                "ccbill",
				"event_source":             "webhook",
				"failure_code":             failureCode,
				"failure_reason":           failureReason,
				"original_subscription_id": originalSubscriptionID,
				"original_client_accnum":   data.OriginalClientAccnum,
				"original_client_subacc":   data.OriginalClientSubacc,
				"upgrade_source":           data.Source,
				"sca_response_status":      data.SCAResponseStatus,
				"card_sub_type":            data.CardSubType,
				"form_name":                data.FormName,
				"flex_id":                  data.FlexID,
			}

			paymentEventData := billing.PaymentEventData{
				EventID:       uuid.New(),
				UserID:        user.ID,
				EventType:     "upgrade_failed",
				Processor:     "ccbill",
				Currency:      "USD",
				BillingInfo:   billing.CreateMetadataJSON(map[string]interface{}{"upgrade_failure": true}),
				WebhookSource: "webhook",
				Metadata:      billing.CreateMetadataJSON(metadata),
				Timestamp:     time.Now().UTC(),
			}

			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
				log.WithError(err).Error("Failed to log upgrade failure event to ClickHouse")
			}
		}

		// Add notification to queue for user about upgrade failure and send immediate email
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    user.ID,
				EventType: models.NotificationPaymentMethodFailed,
			}
			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver upgrade failure notification")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"userID":                 user.ID,
			"email":                  email,
			"failureCode":            failureCode,
			"failureReason":          failureReason,
			"transactionID":          transactionID,
			"originalSubscriptionID": originalSubscriptionID,
		}).Info("Handled upgrade failure")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleBillingDateChange(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing CCBill billing date change notification")

	var data CCBillBillingDateChangeEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	pSubscriptionID := data.SubscriptionID
	nextRenewalDate := data.NextRenewalDate

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
				
		// Find subscription by processor subscription ID
		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), pSubscriptionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", pSubscriptionID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		// Parse the new renewal date
		newRenewalDate, err := time.Parse("2006-01-02 15:04:05", nextRenewalDate)
		if err != nil {
			// Try alternative date format
			newRenewalDate, err = time.Parse("2006-01-02", nextRenewalDate)
			if err != nil {
				return fmt.Errorf("failed to parse nextRenewalDate '%s': %w", nextRenewalDate, err)
			}
		}

		// Update subscription billing date
		sub.CurrentPeriodEndsAt = &newRenewalDate

		if err := s.UpdateSubscription(ctx, sub); err != nil {
			return fmt.Errorf("failed to update subscription billing date: %w", err)
		}

		// Log billing date change event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"processor_subscription_id": pSubscriptionID,
				"processor":                 "ccbill",
				"event_source":              "webhook",
				"old_renewal_date":          sub.CurrentPeriodEndsAt,
				"new_renewal_date":          newRenewalDate,
			}

			subscriptionEventData := billing.SubscriptionEventData{
				EventID:                 uuid.New(),
				SubscriptionID:          sub.ID,
				UserID:                  sub.UserID,
				EventType:               "billing_date_changed",
				Processor:               "ccbill",
				ProcessorSubscriptionID: &pSubscriptionID,
				Currency:                "USD",
				Metadata:                billing.CreateMetadataJSON(metadata),
				Timestamp:               time.Now(),
			}

			if err := s.BillingEventService.LogSubscriptionEvent(ctx, subscriptionEventData); err != nil {
				log.WithError(err).Error("Failed to log billing date change event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID":          sub.ID,
			"userID":                  sub.UserID,
			"processorSubscriptionID": pSubscriptionID,
			"newRenewalDate":          newRenewalDate,
		}).Info("Updated subscription billing date successfully")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleCustomerDataUpdate(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing CCBill customer data update notification")

	var data CCBillCustomerDataUpdateEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	pSubscriptionID := data.SubscriptionID
	email := data.Email

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
						
		// Find subscription by processor subscription ID
		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), pSubscriptionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", pSubscriptionID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		// Get user to validate email change
		user, err := s.GetGoTrueUserByID(ctx, sub.UserID)
		if err != nil {
			return fmt.Errorf("failed to get user for subscription: %w", err)
		}

		// Log customer data update event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"processor_subscription_id": pSubscriptionID,
				"processor":                 "ccbill",
				"event_source":              "webhook",
				"updated_email":             email,
				"previous_email":            user.Email,
				"payment_account":           data.PaymentAccount,
				"card_type":                 data.CardType,
				"payment_type":              data.PaymentType,
				"bin":                       data.Bin,
				"exp_date":                  data.ExpDate,
				"updated_fields": map[string]interface{}{
					"firstName":   data.FirstName,
					"lastName":    data.LastName,
					"address1":    data.Address1,
					"city":        data.City,
					"state":       data.State,
					"country":     data.Country,
					"postalCode":  data.PostalCode,
					"phoneNumber": data.PhoneNumber,
				},
			}

			subscriptionEventData := billing.SubscriptionEventData{
				EventID:                 uuid.New(),
				SubscriptionID:          sub.ID,
				UserID:                  sub.UserID,
				EventType:               "customer_data_updated",
				Processor:               "ccbill",
				ProcessorSubscriptionID: &pSubscriptionID,
				Currency:                "USD",
				Metadata:                billing.CreateMetadataJSON(metadata),
				Timestamp:               time.Now(),
			}

			if err := s.BillingEventService.LogSubscriptionEvent(ctx, subscriptionEventData); err != nil {
				log.WithError(err).Error("Failed to log customer data update event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID":          sub.ID,
			"userID":                  sub.UserID,
			"processorSubscriptionID": pSubscriptionID,
			"updatedEmail":            email,
			"paymentAccount":          data.PaymentAccount,
		}).Info("Processed customer data update successfully")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleUserReactivation(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing CCBill user reactivation notification")

	var data CCBillUserReactivationEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	pSubscriptionID := data.SubscriptionID
	transactionID := data.TransactionID
	email := data.Email
	priceStr := data.Price
	nextRenewalDate := data.NextRenewalDate

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
				
		// Note: We could validate that the email matches the subscription's user email here
		// but for now we'll rely on the subscription lookup

		// Find subscription by processor subscription ID
		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), pSubscriptionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", pSubscriptionID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		// Parse next renewal date if provided
		var renewalDate *time.Time
		if nextRenewalDate != "" {
			parsed, err := time.Parse("2006-01-02", nextRenewalDate)
			if err != nil {
				log.WithContext(ctx).WithError(err).Warn("Failed to parse nextRenewalDate for reactivation")
			} else {
				renewalDate = &parsed
			}
		}

		// Reactivate subscription
		now := time.Now()
		sub.Status = models.StatusActive
		sub.CancelledAt = nil
		sub.CancelType = nil
		sub.CancelFeedback = nil
		sub.EndedAt = nil

		if renewalDate != nil {
			sub.CurrentPeriodEndsAt = renewalDate
			sub.CurrentPeriodStartsAt = &now
		}

		if err := s.UpdateSubscription(ctx, sub); err != nil {
			return fmt.Errorf("failed to reactivate subscription: %w", err)
		}

		// Grant role for reactivated subscription
		if sub.Price != nil {
			grantParams := newGrantRoleParams(sub.UserID, sub.ID, models.ProcessorCCBill, sub.Price, sub.Price.Product, s)
			if err := grantRole(ctx, grantParams); err != nil {
				return fmt.Errorf("failed to grant role for reactivated subscription: %w", err)
			}
		}

		// Log reactivation event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"transaction_id":            transactionID,
				"processor_subscription_id": pSubscriptionID,
				"processor":                 "ccbill",
				"event_source":              "webhook",
				"price_description":         priceStr,
				"next_renewal_date":         nextRenewalDate,
				"reactivation_type":         "user_initiated",
			}

			subscriptionEventData := billing.SubscriptionEventData{
				EventID:                 uuid.New(),
				SubscriptionID:          sub.ID,
				UserID:                  sub.UserID,
				EventType:               "subscription_reactivated",
				Processor:               "ccbill",
				ProcessorSubscriptionID: &pSubscriptionID,
				Currency:                "USD",
				Metadata:                billing.CreateMetadataJSON(metadata),
				Timestamp:               time.Now(),
			}

			if err := s.BillingEventService.LogSubscriptionEvent(ctx, subscriptionEventData); err != nil {
				log.WithError(err).Error("Failed to log reactivation event to ClickHouse")
			}
		}

		// Add notification to queue for user about reactivation and send immediate email
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    sub.UserID,
				EventType: models.NotificationPremiumStarted, // Use started for reactivations
			}
			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver reactivation notification")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID":          sub.ID,
			"userID":                  sub.UserID,
			"transactionID":           transactionID,
			"processorSubscriptionID": pSubscriptionID,
			"email":                   email,
			"priceDescription":        priceStr,
			"nextRenewalDate":         nextRenewalDate,
		}).Info("Processed user reactivation successfully")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleRefund(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing CCBill refund notification")

	var data CCBillRefundEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	pSubscriptionID := data.SubscriptionID
	refundAmountStr := data.Amount
	refundTransactionID := data.TransactionID // Use TransactionID as the refund transaction ID
	refundReason := data.Reason

	// Parse the refund amount
	refundAmount, err := strconv.ParseFloat(refundAmountStr, 64)
	if err != nil {
		return fmt.Errorf("failed to parse refund amount '%s': %w", refundAmountStr, err)
	}

	if refundAmount <= 0 {
		return fmt.Errorf("invalid amount: %f - must be greater than 0", refundAmount)
	}

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
						
		// Find subscription by processor subscription ID
		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), pSubscriptionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", pSubscriptionID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		// Determine if we should terminate the subscription based on refund type
		shouldTerminate := false
		// Determine if we should terminate the subscription based on refund amount
		// If refund amount is significant relative to the subscription price, terminate
		if sub.Price != nil {
			refundPercentage := refundAmount / sub.Price.Amount
			if refundPercentage >= 0.8 { // If refund is 80%+ of price, terminate
				shouldTerminate = true
			}
		}

		now := time.Now()

		if shouldTerminate {
			// Terminate the subscription
			if err := sub.ResetCurrentPeriods(); err != nil {
				return err
			}

			cancelType := models.CancelTypeMerchant // Refund is merchant-initiated
			sub.Status = models.StatusCancelled
			sub.CancelType = &cancelType
			sub.CancelledAt = &now
			if refundReason != "" {
				sub.CancelFeedback = &refundReason
			}

			// Revoke all role grants for this subscription
			if err := s.RevokeUserRolesBySubSourceID(ctx, sub.ID); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to revoke role grants for refunded subscription")
			}

			// Add notification to queue for user about account termination due to refund
			if s.NotificationService != nil {
				notification := &models.NotificationQueue{
					ID:        uuid.New(),
					UserID:    sub.UserID,
					EventType: models.NotificationPremiumEnded,
				}
				if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
					log.WithContext(ctx).WithError(err).Error("failed to create and deliver refund termination notification")
				}
			}
		} else {
			// Don't terminate, but log the refund for record keeping
			log.WithContext(ctx).WithFields(log.Fields{
				"subscriptionID":      sub.ID,
				"refundAmount":        refundAmount,
				"refundType":          "auto_detected",
				"refundTransactionID": refundTransactionID,
			}).Info("Partial refund processed - subscription remains active")
		}

		if err := s.UpdateSubscription(ctx, sub); err != nil {
			return fmt.Errorf("failed to update subscription after refund: %w", err)
		}

		// Log refund event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"refund_transaction_id":     refundTransactionID,
				"processor":                 "ccbill",
				"event_source":              "webhook",
				"refund_reason":             refundReason,
				"refund_type":               "auto_detected",
				"refund_amount":             refundAmount,
				"subscription_terminated":   shouldTerminate,
				"processor_subscription_id": pSubscriptionID,
			}

			// Log as payment event (negative amount for refund)
			negativeAmount := -refundAmount
			paymentEventData := billing.PaymentEventData{
				EventID:        uuid.New(),
				SubscriptionID: &sub.ID,
				UserID:         sub.UserID,
				EventType:      "refund",
				Processor:      "ccbill",
				Amount:         &negativeAmount,
				Currency:       "USD",
				BillingInfo:    billing.CreateMetadataJSON(map[string]interface{}{"refund": true}),
				WebhookSource:  "webhook",
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now().UTC(),
			}

			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
				log.WithError(err).Error("Failed to log refund event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID":         sub.ID,
			"userID":                 sub.UserID,
			"refundAmount":           refundAmount,
			"refundType":             "auto_detected",
			"refundTransactionID":    refundTransactionID,
			"subscriptionTerminated": shouldTerminate,
		}).Info("Processed refund successfully")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleVoid(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing CCBill void notification")

	var data CCBillVoidEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	pSubscriptionID := data.SubscriptionID
	voidAmountStr := data.Amount
	voidTransactionID := data.TransactionID // Use TransactionID as the void transaction ID
	voidReason := data.Reason

	// Parse the void amount
	voidAmount, err := strconv.ParseFloat(voidAmountStr, 64)
	if err != nil {
		return fmt.Errorf("failed to parse void amount '%s': %w", voidAmountStr, err)
	}

	if voidAmount <= 0 {
		return fmt.Errorf("invalid amount: %f - must be greater than 0", voidAmount)
	}

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
				
		// Try to find subscription by processor subscription ID
		// Note: For voids, the subscription might not exist yet since the transaction was voided
		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), pSubscriptionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// This is expected for voids - the subscription may never have been created
				log.WithContext(ctx).WithFields(log.Fields{
					"processor_subscription_id": pSubscriptionID,
					"void_amount":               voidAmount,
					"void_transaction_id":       voidTransactionID,
					"original_transaction_id":   voidTransactionID,
				}).Info("Void event for non-existent subscription - transaction was voided before subscription creation")

				// Still log the void event for audit purposes, but without subscription ID
				if s.BillingEventService != nil {
					metadata := map[string]interface{}{
						"void_transaction_id":       voidTransactionID,
						"original_transaction_id":   voidTransactionID,
						"processor":                 "ccbill",
						"event_source":              "webhook",
						"void_reason":               voidReason,
						"void_amount":               voidAmount,
						"processor_subscription_id": pSubscriptionID,
						"subscription_exists":       false,
					}

					// Log as payment event (negative amount for void)
					negativeAmount := -voidAmount
					paymentEventData := billing.PaymentEventData{
						EventID:       uuid.New(),
						EventType:     "void",
						Processor:     "ccbill",
						Amount:        &negativeAmount,
						Currency:      "USD",
						BillingInfo:   billing.CreateMetadataJSON(map[string]interface{}{"void": true}),
						WebhookSource: "webhook",
						Metadata:      billing.CreateMetadataJSON(metadata),
						Timestamp:     time.Now().UTC(),
					}

					if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
						log.WithError(err).Error("Failed to log void event to ClickHouse")
					}
				}

				return nil // Don't fail webhook processing
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		// Subscription exists - void doesn't terminate it, just log the event
		// Voids typically happen before settlement, so the subscription remains as-is
		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID":        sub.ID,
			"userID":                sub.UserID,
			"voidAmount":            voidAmount,
			"voidTransactionID":     voidTransactionID,
			"originalTransactionID": voidTransactionID,
		}).Info("Void event for existing subscription - no subscription changes made")

		// Log void event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"void_transaction_id":       voidTransactionID,
				"original_transaction_id":   voidTransactionID,
				"processor":                 "ccbill",
				"event_source":              "webhook",
				"void_reason":               voidReason,
				"void_amount":               voidAmount,
				"processor_subscription_id": pSubscriptionID,
				"subscription_exists":       true,
			}

			// Log as payment event (negative amount for void)
			negativeAmount := -voidAmount
			paymentEventData := billing.PaymentEventData{
				EventID:        uuid.New(),
				SubscriptionID: &sub.ID,
				UserID:         sub.UserID,
				EventType:      "void",
				Processor:      "ccbill",
				Amount:         &negativeAmount,
				Currency:       "USD",
				BillingInfo:    billing.CreateMetadataJSON(map[string]interface{}{"void": true}),
				WebhookSource:  "webhook",
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now().UTC(),
			}

			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
				log.WithError(err).Error("Failed to log void event to ClickHouse")
			}
		}

		// No subscription modifications needed for voids - they're just transaction cleanup
		// The subscription remains in its current state

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID":    sub.ID,
			"userID":            sub.UserID,
			"voidAmount":        voidAmount,
			"voidTransactionID": voidTransactionID,
		}).Info("Processed void successfully - no subscription changes")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleChargeback(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Warn("Processing CCBill chargeback notification - immediate termination required")

	var data CCBillChargebackEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	pSubscriptionID := data.SubscriptionID
	chargebackAmountStr := data.Amount
	chargebackTransactionID := data.TransactionID // Use TransactionID as the chargeback transaction ID
	chargebackReason := data.Reason

	// Parse the chargeback amount
	chargebackAmount, err := strconv.ParseFloat(chargebackAmountStr, 64)
	if err != nil {
		return fmt.Errorf("failed to parse chargeback amount '%s': %w", chargebackAmountStr, err)
	}

	if chargebackAmount <= 0 {
		return fmt.Errorf("invalid amount: %f - must be greater than 0", chargebackAmount)
	}

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
								
		// Find subscription by processor subscription ID
		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), pSubscriptionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.WithContext(ctx).WithFields(log.Fields{
					"processor_subscription_id": pSubscriptionID,
					"chargeback_amount":         chargebackAmount,
					"dispute_id":                "unknown",
				}).Error("Chargeback event for non-existent subscription - potential fraud")

				// Still log the chargeback event for audit purposes
				if s.BillingEventService != nil {
					metadata := map[string]interface{}{
						"chargeback_transaction_id": chargebackTransactionID,
						"original_transaction_id":   chargebackTransactionID,
						"processor":                 "ccbill",
						"event_source":              "webhook",
						"chargeback_reason":         chargebackReason,
						"chargeback_reason_code":    "unknown",
						"chargeback_amount":         chargebackAmount,
						"dispute_id":                "unknown",
						"processor_subscription_id": pSubscriptionID,
						"subscription_exists":       false,
						"fraud_flag":                true,
					}

					// Log as payment event (negative amount for chargeback)
					negativeAmount := -chargebackAmount
					paymentEventData := billing.PaymentEventData{
						EventID:       uuid.New(),
						EventType:     "chargeback",
						Processor:     "ccbill",
						Amount:        &negativeAmount,
						Currency:      "USD",
						BillingInfo:   billing.CreateMetadataJSON(map[string]interface{}{"chargeback": true, "fraud_flag": true}),
						WebhookSource: "webhook",
						Metadata:      billing.CreateMetadataJSON(metadata),
						Timestamp:     time.Now().UTC(),
					}

					if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
						log.WithError(err).Error("Failed to log chargeback event to ClickHouse")
					}
				}

				return nil // Don't fail webhook processing
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		// Get user for fraud flagging
		user, err := s.GetGoTrueUserByID(ctx, sub.UserID)
		if err != nil {
			log.WithContext(ctx).WithError(err).WithField("user_id", sub.UserID).Error("failed to get user for chargeback processing")
			// Continue with subscription termination even if user lookup fails
		}

		now := time.Now()

		// IMMEDIATE TERMINATION - chargebacks are the most serious type of dispute
		if err := sub.ResetCurrentPeriods(); err != nil {
			return err
		}

		// Mark as merchant cancellation due to chargeback
		cancelType := models.CancelTypeMerchant
		sub.Status = models.StatusCancelled
		sub.CancelType = &cancelType
		sub.CancelledAt = &now
		sub.EndedAt = &now

		// Include chargeback details in feedback
		chargebackFeedback := fmt.Sprintf("CHARGEBACK: %s (Code: %s, Dispute: %s)",
			chargebackReason, "unknown", "unknown")
		sub.CancelFeedback = &chargebackFeedback

		if err := s.UpdateSubscription(ctx, sub); err != nil {
			return fmt.Errorf("failed to update subscription after chargeback: %w", err)
		}

		// Immediately revoke all role grants for this subscription
		if err := s.RevokeUserRolesBySubSourceID(ctx, sub.ID); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to revoke role grants for chargebacked subscription")
		}

		// TODO: Add fraud flagging logic here
		// This could include:
		// 1. Flagging user account for manual review
		// 2. Adding to fraud prevention database
		// 3. Blocking future payments from same user/card/IP
		// 4. Alerting fraud prevention team

		if user != nil {
			log.WithContext(ctx).WithFields(log.Fields{
				"user_id":                user.ID,
				"user_email":             user.Email,
				"chargeback_amount":      chargebackAmount,
				"dispute_id":             "unknown",
				"chargeback_reason_code": "unknown",
			}).Warn("User account involved in chargeback - consider fraud review")
		}

		// Log chargeback event to ClickHouse with fraud flags
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"chargeback_transaction_id": chargebackTransactionID,
				"original_transaction_id":   chargebackTransactionID,
				"processor":                 "ccbill",
				"event_source":              "webhook",
				"chargeback_reason":         chargebackReason,
				"chargeback_reason_code":    "unknown",
				"chargeback_amount":         chargebackAmount,
				"dispute_id":                "unknown",
				"processor_subscription_id": pSubscriptionID,
				"subscription_terminated":   true,
				"fraud_flag":                true,
				"termination_immediate":     true,
			}

			// Log as payment event (negative amount for chargeback)
			negativeAmount := -chargebackAmount
			paymentEventData := billing.PaymentEventData{
				EventID:        uuid.New(),
				SubscriptionID: &sub.ID,
				UserID:         sub.UserID,
				EventType:      "chargeback",
				Processor:      "ccbill",
				Amount:         &negativeAmount,
				Currency:       "USD",
				BillingInfo:    billing.CreateMetadataJSON(map[string]interface{}{"chargeback": true, "fraud_flag": true}),
				WebhookSource:  "webhook",
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now().UTC(),
			}

			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
				log.WithError(err).Error("Failed to log chargeback event to ClickHouse")
			}
		}

		// Add system alert notification for chargeback (admin notification)
		if s.NotificationService != nil {
			// User notification about account termination
			userNotification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    sub.UserID,
				EventType: models.NotificationPremiumEnded,
			}
			if err := s.NotificationService.CreateAndDeliver(ctx, userNotification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver chargeback termination notification")
			}

			// TODO: Add system alert notification for fraud team
			// This would be a different notification type for internal alerts
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID":          sub.ID,
			"userID":                  sub.UserID,
			"chargebackAmount":        chargebackAmount,
			"chargebackTransactionID": chargebackTransactionID,
			"chargebackReasonCode":    "unknown",
			"disputeID":               "unknown",
			"subscriptionTerminated":  true,
			"fraudFlag":               true,
		}).Error("Processed chargeback - subscription terminated immediately")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleRenewalSuccess(ctx context.Context) error {
	log.WithContext(ctx).
		WithField("eventType", s.Data.EventType).
		Info("Processing CCBill renewal success notification")

	var data CCBillRenewalSuccessEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	ccBillSubID := data.SubscriptionID
	transactionID := data.TransactionID
	billedAmountStr := data.BilledAmount

	billedAmount, err := strconv.ParseFloat(billedAmountStr, 64)
	if err != nil {
		return err
	}

	if billedAmount <= 0 {
		return fmt.Errorf("invalid billedAmount: %f - must be greater than 0", billedAmount)
	}

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
						
		subscription, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), ccBillSubID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", ccBillSubID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		price := subscription.Price
		if price == nil {
			price, err = s.GetPriceByID(ctx, subscription.PriceID)
			if err != nil {
				return fmt.Errorf("failed to find new price for ID %s: %w", price.ID, err)
			}
		}

		// Validate the billed amount matches the new price
		expectedAmount := price.Amount
		tolerance := expectedAmount * 0.02
		if billedAmount < (expectedAmount-tolerance) || billedAmount > (expectedAmount+tolerance) {
			billingErr := newBillingError(ErrorTypeAmount,
				"Renewal amount does not match expected price",
				map[string]interface{}{
					"expected_amount": expectedAmount,
					"billed_amount":   billedAmount,
					"tolerance":       tolerance,
					"price_id":        price.ID.String(),
				}, nil)

			s.logBillingError(ctx, billingErr, log.Fields{
				"transaction_id":  transactionID,
				"subscription_id": subscription.ID,
			})
			return billingErr
		}

		if err := subscription.ActivateWithPrice(price); err != nil {
			return fmt.Errorf("failed to activate subscription: %w", err)
		}

		if err := subscription.Validate(billedAmount); err != nil {
			return fmt.Errorf("failed to validate subscription: %w", err)
		}

		if err := s.UpdateSubscription(ctx, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		if err := grantRole(
			ctx,
			newGrantRoleParams(subscription.UserID, subscription.ID, models.ProcessorCCBill, price, price.Product, s),
		); err != nil {
			return fmt.Errorf("failed to grant role for renewed subscription: %w", err)
		}

		// Log renewal payment event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"transaction_id":  transactionID,
				"processor":       "ccbill",
				"event_source":    "webhook",
				"event_type":      "renewal",
				"amount":          billedAmount,
				"subscription_id": subscription.ID,
				"price_id":        price.ID.String(),
			}

			paymentEventData := billing.PaymentEventData{
				EventID:        uuid.New(),
				SubscriptionID: &subscription.ID,
				UserID:         subscription.UserID,
				EventType:      "charge_success",
				Processor:      "ccbill",
				Amount:         &billedAmount,
				Currency:       "USD",
				BillingInfo:    billing.CreateMetadataJSON(map[string]interface{}{"renewal": true}),
				WebhookSource:  "webhook",
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now().UTC(),
			}

			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
				log.WithError(err).Error("Failed to log renewal payment event to ClickHouse")
			}
		}

		// Add notification to queue for user about successful renewal and send immediate email
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    subscription.UserID,
				EventType: models.NotificationPremiumRenewed,
			}
			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver renewal success notification")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID": subscription.ID,
			"userID":         subscription.UserID,
			"priceID":        price.ID,
			"billedAmount":   billedAmount,
			"transactionID":  transactionID,
		}).Info("Processed subscription renewal successfully")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleRenewalFailure(ctx context.Context) error {
	var data CCBillRenewalFailureEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	ccBillSubID := data.SubscriptionID

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
				
		// Find subscription by processor subscription ID
		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), ccBillSubID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", ccBillSubID)
			}

			return fmt.Errorf("failed to get subscription: %w", err)
		}

		if err := sub.ResetCurrentPeriods(); err != nil {
			return err
		}

		now := time.Now()
		if sub.RetryAttempts == nil {
			attempts := 1
			sub.RetryAttempts = &attempts
		} else {
			*sub.RetryAttempts++
		}
		sub.LastRetryAt = &now

		if err := s.UpdateSubscription(ctx, sub); err != nil {
			return err
		}

		// DO NOT revoke role grants - user retains access during grace period (past_due status)
		// Role grants will only be revoked when subscription moves to StatusCancelled

		// Add notification to queue for user about payment failure and send immediate email
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    sub.UserID,
				EventType: models.NotificationPaymentMethodFailed,
			}
			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver payment failed notification")
			}
		}

		// Log payment failure event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"processor_subscription_id": ccBillSubID,
				"failure_code":              data.FailureCode,
				"failure_reason":            data.FailureReason,
				"retry_attempts":            sub.RetryAttempts,
				"subscription_status":       string(sub.Status), // past_due
			}

			paymentEventData := billing.PaymentEventData{
				EventID:        uuid.New(),
				SubscriptionID: &sub.ID,
				UserID:         sub.UserID,
				EventType:      "charge_failed",
				Processor:      "ccbill",
				Currency:       "USD",
				BillingInfo:    billing.CreateMetadataJSON(map[string]interface{}{}),
				WebhookSource:  "webhook",
				Metadata:       billing.CreateMetadataJSON(metadata),
				Timestamp:      time.Now().UTC(),
			}

			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
				log.WithError(err).Error("Failed to log payment failure event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID": sub.ID,
			"userID":         sub.UserID,
			"failureCode":    data.FailureCode,
			"failureReason":  data.FailureReason,
		}).Info("Handled renewal failure")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleCancel(ctx context.Context) error {
	var data CCBillCancellationEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	ccBillSubID := data.SubscriptionID
	if ccBillSubID == "" {
		return fmt.Errorf("missing required field: subscriptionId")
	}

	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
						
		// Find subscription by processor subscription ID
		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), ccBillSubID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", ccBillSubID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		// Determine cancel type based on source
		var cancelType models.CancelType
		if data.Source == "failedRB" {
			cancelType = models.CancelTypeExpired
		} else {
			cancelType = models.CancelTypeMerchant
		}

		if err := sub.Cancel(data.Reason, &cancelType); err != nil {
			return err
		}

		if err := s.UpdateSubscription(ctx, sub); err != nil {
			return err
		}

		// Revoke all role grants for this subscription
		if err := s.RevokeUserRolesBySubSourceID(ctx, sub.ID); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to revoke role grants for cancelled subscription")
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
				"processor_subscription_id": ccBillSubID,
				"cancel_reason":             data.Reason,
				"cancel_source":             data.Source,
				"cancel_type":               string(cancelType),
				"is_failed_rebill":          data.Source == "failedRB",
			}

			subscriptionEventData := billing.SubscriptionEventData{
				EventID:                 uuid.New(),
				SubscriptionID:          sub.ID,
				UserID:                  sub.UserID,
				EventType:               "subscription_cancelled",
				Processor:               "ccbill",
				ProcessorSubscriptionID: &ccBillSubID,
				Currency:                "USD",
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
			"processorSubscriptionID": ccBillSubID,
			"cancelReason":            data.Reason,
			"cancelSource":            data.Source,
		}).Info("Cancelled subscription successfully")

		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *CCBillWebhookService) handleExpiration(ctx context.Context) error {
	var data CCBillExpirationEvent
	if err := json.Unmarshal(s.Data.EventBody, &data); err != nil {
		return err
	}

	ccBillSubID := data.SubscriptionID
	if err := s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
						
		// Find subscription by processor subscription ID
		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorCCBill), ccBillSubID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription not found for processor subscription ID: %s", ccBillSubID)
			}
			return fmt.Errorf("failed to get subscription: %w", err)
		}

		cancelType := models.CancelTypeExpired
		if err := sub.Cancel("", &cancelType); err != nil {
			return err
		}

		if err := s.UpdateSubscription(ctx, sub); err != nil {
			return err
		}

		// Revoke all role grants for this subscription
		if err := s.RevokeUserRolesBySubSourceID(ctx, sub.ID); err != nil {
			log.WithContext(ctx).WithError(err).Error("failed to revoke role grants for expired subscription")
		}

		// Add notification to queue for user and send immediate email
		if s.NotificationService != nil {
			notification := &models.NotificationQueue{
				ID:        uuid.New(),
				UserID:    sub.UserID,
				EventType: models.NotificationPremiumEnded,
			}

			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
				log.WithContext(ctx).WithError(err).Error("failed to create and deliver membership expired notification")
			}
		}

		// Log subscription expiration event to ClickHouse
		if s.BillingEventService != nil {
			metadata := map[string]interface{}{
				"processor_subscription_id": ccBillSubID,
				"cancel_source":             "expiration",
				"cancel_type":               string(cancelType),
				"is_expiration":             true,
			}

			subscriptionEventData := billing.SubscriptionEventData{
				EventID:                 uuid.New(),
				SubscriptionID:          sub.ID,
				UserID:                  sub.UserID,
				EventType:               "subscription_expired",
				Processor:               "ccbill",
				ProcessorSubscriptionID: &ccBillSubID,
				Currency:                "USD",
				Metadata:                billing.CreateMetadataJSON(metadata),
				Timestamp:               time.Now(),
			}

			if err := s.BillingEventService.LogSubscriptionEvent(ctx, subscriptionEventData); err != nil {
				log.WithError(err).Error("Failed to log subscription expiration event to ClickHouse")
			}
		}

		log.WithContext(ctx).WithFields(log.Fields{
			"subscriptionID":          sub.ID,
			"userID":                  sub.UserID,
			"processorSubscriptionID": ccBillSubID,
		}).Info("Expired subscription successfully")

		return nil
	}); err != nil {
		return err
	}

	return nil
}
