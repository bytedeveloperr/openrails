package services

const MobiusProcessorName string = "Mobius"

// type MobiusWebhookService struct {
// 	DB                   *db.DB
// 	Data                 MobiusWebhookEvent
// 	MobiusClient         *mobius.MobiusClient
// 	DeadLetterService    *webhook.DeadLetterService
// 	NotificationService  *notification.NotificationService
// 	BillingEventService  *billing.BillingEventService
// 	DeduplicationService *webhook.DeduplicationService
// }

// // Repository methods for MobiusWebhookService

// // User repository methods
// func (s *MobiusWebhookService) GetGoTrueUserByEmail(ctx context.Context, email string) (*types.User, error) {
// 	// This would typically call the GoTrue API or database
// 	// For now, return a placeholder implementation
// 	return nil, fmt.Errorf("user not found: %s", email)
// }

// // Price repository methods
// func (s *MobiusWebhookService) GetPriceByMobiusPlanID(ctx context.Context, planID string) (*models.Price, error) {
// 	var price models.Price
// 	err := s.DB.GetDB().NewSelect().
// 		Model(&price).
// 		Where("mobius_plan_id = ?", planID).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get price by Mobius plan ID: %w", err)
// 	}
// 	return &price, nil
// }

// // Subscription repository methods
// func (s *MobiusWebhookService) GetSubscriptionByUserIDAndPriceID(ctx context.Context, userID, priceID uuid.UUID) (*models.Subscription, error) {
// 	var subscription models.Subscription
// 	err := s.DB.GetDB().NewSelect().
// 		Model(&subscription).
// 		Where("user_id = ?", userID).
// 		Where("price_id = ?", priceID).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get subscription by user ID and price ID: %w", err)
// 	}
// 	return &subscription, nil
// }

// func (s *MobiusWebhookService) GetSubscriptionByProcessorSubscriptionID(ctx context.Context, processor, processorSubID string) (*models.Subscription, error) {
// 	var subscription models.Subscription
// 	err := s.DB.GetDB().NewSelect().
// 		Model(&subscription).
// 		Where("processor = ?", processor).
// 		Where("processor_subscription_id = ?", processorSubID).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get subscription by processor subscription ID: %w", err)
// 	}
// 	return &subscription, nil
// }

// func (s *MobiusWebhookService) CreateSubscription(ctx context.Context, subscription *models.Subscription) error {
// 	_, err := s.DB.GetDB().NewInsert().
// 		Model(subscription).
// 		Exec(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to create subscription: %w", err)
// 	}
// 	return nil
// }

// func (s *MobiusWebhookService) UpdateSubscription(ctx context.Context, subscription *models.Subscription) error {
// 	_, err := s.DB.GetDB().NewUpdate().
// 		Model(subscription).
// 		Where("id = ?", subscription.ID).
// 		Exec(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to update subscription: %w", err)
// 	}
// 	return nil
// }

// // Purchase repository methods
// func (s *MobiusWebhookService) GetPurchaseByTransactionID(ctx context.Context, processor models.ProcessorType, transactionID string) (*models.Purchase, error) {
// 	var purchase models.Purchase
// 	err := s.DB.GetDB().NewSelect().
// 		Model(&purchase).
// 		Where("processor = ?", processor).
// 		Where("transaction_id = ?", transactionID).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get purchase by transaction ID: %w", err)
// 	}
// 	return &purchase, nil
// }

// func (s *MobiusWebhookService) CreatePurchase(ctx context.Context, purchase *models.Purchase) error {
// 	_, err := s.DB.GetDB().NewInsert().
// 		Model(purchase).
// 		Exec(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to create purchase: %w", err)
// 	}
// 	return nil
// }

// // Product repository methods
// func (s *MobiusWebhookService) GetProductByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
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

// // UserRoleGrant repository methods
// func (s *MobiusWebhookService) ExtendRoleExpiration(ctx context.Context, userID, roleID uuid.UUID, days int) (*models.UserRoleGrant, time.Time, error) {
// 	// Check if user already has this role
// 	var existingGrant models.UserRoleGrant
// 	err := s.DB.GetDB().NewSelect().
// 		Model(&existingGrant).
// 		Where("user_id = ?", userID).
// 		Where("role_id = ?", roleID).
// 		Where("revoked_at IS NULL").
// 		Scan(ctx)

// 	var newExpirationDate time.Time
// 	if err == nil {
// 		// User has existing grant, extend it
// 		if existingGrant.ExpiresAt != nil {
// 			newExpirationDate = existingGrant.ExpiresAt.AddDate(0, 0, days)
// 		} else {
// 			newExpirationDate = time.Now().AddDate(0, 0, days)
// 		}
// 		existingGrant.ExpiresAt = &newExpirationDate

// 		_, updateErr := s.DB.GetDB().NewUpdate().
// 			Model(&existingGrant).
// 			Where("id = ?", existingGrant.ID).
// 			Exec(ctx)
// 		if updateErr != nil {
// 			return nil, time.Time{}, fmt.Errorf("failed to update role expiration: %w", updateErr)
// 		}
// 		return &existingGrant, newExpirationDate, nil
// 	} else {
// 		// Create new role grant
// 		newExpirationDate = time.Now().AddDate(0, 0, days)
// 		newGrant := &models.UserRoleGrant{
// 			ID:        uuid.New(),
// 			UserID:    userID,
// 			RoleID:    roleID,
// 			ExpiresAt: &newExpirationDate,
// 			CreatedAt: time.Now(),
// 			UpdatedAt: time.Now(),
// 		}

// 		_, insertErr := s.DB.GetDB().NewInsert().
// 			Model(newGrant).
// 			Exec(ctx)
// 		if insertErr != nil {
// 			return nil, time.Time{}, fmt.Errorf("failed to create role grant: %w", insertErr)
// 		}
// 		return newGrant, newExpirationDate, nil
// 	}
// }

// // PaymentMethod repository methods
// func (s *MobiusWebhookService) GetPaymentMethodByVaultID(ctx context.Context, processor models.ProcessorType, vaultID string) (*models.PaymentMethod, error) {
// 	var paymentMethod models.PaymentMethod
// 	err := s.DB.GetDB().NewSelect().
// 		Model(&paymentMethod).
// 		Where("processor = ?", processor).
// 		Where("vault_id = ?", vaultID).
// 		Scan(ctx)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get payment method by vault ID: %w", err)
// 	}
// 	return &paymentMethod, nil
// }

// func (s *MobiusWebhookService) UpdatePaymentMethod(ctx context.Context, paymentMethod *models.PaymentMethod) error {
// 	_, err := s.DB.GetDB().NewUpdate().
// 		Model(paymentMethod).
// 		Where("id = ?", paymentMethod.ID).
// 		Exec(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to update payment method: %w", err)
// 	}
// 	return nil
// }

// // Additional methods needed for RoleGrantService interface
// func (s *MobiusWebhookService) UpdatePurchase(ctx context.Context, purchase *models.Purchase) error {
// 	_, err := s.DB.GetDB().NewUpdate().
// 		Model(purchase).
// 		Where("id = ?", purchase.ID).
// 		Exec(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to update purchase: %w", err)
// 	}
// 	return nil
// }

// type MobiusWebhookEventType = string

// const (
// 	// Subscription lifecycle events
// 	EventTypeMobiusAddSubscription    MobiusWebhookEventType = "recurring.subscription.add"
// 	EventTypeMobiusUpdateSubscription MobiusWebhookEventType = "recurring.subscription.update"
// 	EventTypeMobiusDeleteSubscription MobiusWebhookEventType = "recurring.subscription.delete"

// 	// Transaction events
// 	EventTypeMobiusTransactionSuccess MobiusWebhookEventType = "transaction.sale.success"

// 	// Automatic Card Updater (ACU) events
// 	EventTypeMobiusACUUpdated         MobiusWebhookEventType = "acu.summary.automaticallyupdated"
// 	EventTypeMobiusACUContactCustomer MobiusWebhookEventType = "acu.summary.contactcustomer"
// 	EventTypeMobiusACUClosedAccount   MobiusWebhookEventType = "acu.summary.closedaccount"

// 	// Chargeback events
// 	EventTypeMobiusChargebackComplete MobiusWebhookEventType = "chargeback.batch.complete"
// )

// type MobiusBillingError struct {
// 	Type    string                 `json:"type"`
// 	Message string                 `json:"message"`
// 	Context map[string]interface{} `json:"context"`
// 	Err     error                  `json:"-"`
// }

// func (be *MobiusBillingError) Error() string {
// 	if be.Err != nil {
// 		return fmt.Sprintf("%s: %s (%v)", be.Type, be.Message, be.Err)
// 	}
// 	return fmt.Sprintf("%s: %s", be.Type, be.Message)
// }

// func (be *MobiusBillingError) Unwrap() error {
// 	return be.Err
// }

// const (
// 	ErrorTypeMobiusValidation    = "validation_error"
// 	ErrorTypeMobiusAmount        = "amount_mismatch"
// 	ErrorTypeMobiusDuplicate     = "duplicate_transaction"
// 	ErrorTypeMobiusStatusChange  = "invalid_status_change"
// 	ErrorTypeMobiusBusinessLogic = "business_logic_error"
// 	ErrorTypeMobiusDatabase      = "database_error"
// 	ErrorTypeMobiusNotFound      = "not_found"
// )

// func newMobiusBillingError(errorType string, message string, context map[string]interface{}, err error) *MobiusBillingError {
// 	return &MobiusBillingError{
// 		Type:    errorType,
// 		Message: message,
// 		Context: context,
// 		Err:     err,
// 	}
// }

// func (s *MobiusWebhookService) logMobiusBillingError(ctx context.Context, billingErr *MobiusBillingError, logFields log.Fields) {
// 	fields := log.Fields{
// 		"error_type":    billingErr.Type,
// 		"error_message": billingErr.Message,
// 		"error_context": billingErr.Context,
// 	}

// 	for k, v := range logFields {
// 		fields[k] = v
// 	}

// 	log.WithContext(ctx).WithFields(fields).Error("Mobius billing error occurred")
// }

// func (s *MobiusWebhookService) HandleMobiusWebhook(ctx context.Context) error {
// 	// Use deduplication service if available
// 	if s.DeduplicationService != nil {
// 		return s.DeduplicationService.ProcessWebhook(
// 			ctx,
// 			s.Data.EventID,
// 			s.Data.EventType,
// 			models.ProcessorMobius,
// 			s.Data,
// 			s.handleWebhook,
// 		)
// 	}

// 	return s.handleWebhook(ctx)
// }

// func (s *MobiusWebhookService) handleWebhook(ctx context.Context) error {
// 	switch s.Data.EventType {
// 	// Subscription lifecycle events
// 	case EventTypeMobiusAddSubscription:
// 		return s.handleAddSubscription(ctx)
// 	case EventTypeMobiusUpdateSubscription:
// 		return s.handleUpdateSubscription(ctx)
// 	case EventTypeMobiusDeleteSubscription:
// 		return s.handleDeleteSubscription(ctx)

// 	// Transaction events
// 	case EventTypeMobiusTransactionSuccess:
// 		return s.handleTransactionSuccess(ctx)

// 	// Automatic Card Updater (ACU) events
// 	case EventTypeMobiusACUUpdated:
// 		return s.handleACUUpdated(ctx)
// 	case EventTypeMobiusACUContactCustomer:
// 		return s.handleACUContactCustomer(ctx)
// 	case EventTypeMobiusACUClosedAccount:
// 		return s.handleACUClosedAccount(ctx)

// 	// Chargeback events
// 	case EventTypeMobiusChargebackComplete:
// 		return s.handleChargebackComplete(ctx)

// 	default:
// 		// Log unknown event to dead letter queue if service is available
// 		if s.DeadLetterService != nil {
// 			dataJSON, err := json.Marshal(s.Data)
// 			if err == nil {
// 				s.DeadLetterService.LogUnknownEvent(ctx, "mobius", s.Data.EventType, json.RawMessage(dataJSON), nil, "")
// 			}
// 		}
// 		return fmt.Errorf("unsupported event type: %s", s.Data.EventType)
// 	}
// }

// func (s *MobiusWebhookService) handleAddSubscription(ctx context.Context) error {
// 	log.WithContext(ctx).
// 		WithField("eventType", s.Data.EventType).
// 		Info("Processing Mobius subscription add notification")

// 	mobiusPlanID := s.Data.EventBody.Plan.ID
// 	mobiusSubID := s.Data.EventBody.SubscriptionID

// 	email := ""
// 	if s.Data.EventBody.BillingAddress != nil {
// 		email = s.Data.EventBody.BillingAddress.Email
// 	}

// 	processor := models.ProcessorMobius

// 	if mobiusPlanID == "" {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing plan ID", map[string]interface{}{
// 			"subscription_id": mobiusSubID,
// 		}, nil)
// 	}

// 	if email == "" {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing email address", map[string]interface{}{
// 			"plan_id":         mobiusPlanID,
// 			"subscription_id": mobiusSubID,
// 		}, nil)
// 	}

// 	if mobiusSubID == "" {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing subscription ID", map[string]interface{}{
// 			"plan_id": mobiusPlanID,
// 			"email":   email,
// 		}, nil)
// 	}

// 	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {

// 		user, err := s.GetGoTrueUserByEmail(ctx, email)
// 		if err != nil {
// 			return fmt.Errorf("failed to find user with email %s: %w", email, err)
// 		}

// 		price, err := s.GetPriceByMobiusPlanID(ctx, mobiusPlanID)
// 		if err != nil {
// 			return fmt.Errorf("failed to find price for Mobius plan ID %s: %w", mobiusPlanID, err)
// 		}

// 		subscription, err := s.GetSubscriptionByUserIDAndPriceID(ctx, user.ID, price.ID)
// 		if err != nil && !errors.Is(err, sql.ErrNoRows) {
// 			return fmt.Errorf("failed to check existing subscription: %w", err)
// 		}

// 		var isNewSubscription bool
// 		if subscription != nil && subscription.Processor == processor {
// 			if subscription.ProcessorSubscriptionID == mobiusSubID {
// 				subscription.ProcessorSubscriptionID = mobiusSubID
// 			}
// 		} else {
// 			isNewSubscription = true
// 			subscription = &models.Subscription{
// 				ID:                      uuid.New(),
// 				UserID:                  user.ID,
// 				Processor:               processor,
// 				StartedAt:               time.Now(),
// 				ProcessorSubscriptionID: mobiusSubID,
// 			}
// 		}

// 		if err := subscription.ActivateWithPrice(price); err != nil {
// 			return fmt.Errorf("failed to activate new subscription: %w", err)
// 		}

// 		if err := subscription.Validate(price.Amount); err != nil {
// 			return fmt.Errorf("failed to validate new subscription: %w", err)
// 		}

// 		if isNewSubscription {
// 			if err := s.CreateSubscription(ctx, subscription); err != nil {
// 				return fmt.Errorf("failed to create subscription: %w", err)
// 			}
// 		} else {
// 			if err := s.UpdateSubscription(ctx, subscription); err != nil {
// 				return fmt.Errorf("failed to update subscription: %w", err)
// 			}
// 		}

// 		if err := grantRole(
// 			ctx,
// 			newGrantRoleParams(user.ID, subscription.ID, processor, price, price.Product, s),
// 		); err != nil {
// 			return fmt.Errorf("failed to grant role: %w", err)
// 		}

// 		if s.BillingEventService != nil {
// 			metadata := map[string]interface{}{
// 				"processor_subscription_id": mobiusSubID,
// 				"plan_id":                   mobiusPlanID,
// 				"amount":                    price.Amount,
// 				"billing_cycle_days":        price.BillingCycleDays,
// 				"period_start":              subscription.CurrentPeriodStartsAt,
// 				"period_end":                subscription.CurrentPeriodEndsAt,
// 			}

// 			subscriptionEventData := billing.SubscriptionEventData{
// 				EventID:                 uuid.New(),
// 				UserID:                  user.ID,
// 				Processor:               "mobius",
// 				Timestamp:               time.Now(),
// 				ProcessorSubscriptionID: &mobiusSubID,
// 				Amount:                  &price.Amount,
// 				Currency:                price.Currency,
// 				SubscriptionID:          subscription.ID,
// 				EventType:               "subscription_created",
// 				Metadata:                billing.CreateMetadataJSON(metadata),
// 			}

// 			if err := s.BillingEventService.LogSubscriptionEvent(ctx, subscriptionEventData); err != nil {
// 				log.WithError(err).Error("Failed to log subscription creation event to ClickHouse")
// 			}
// 		}

// 		return nil
// 	})
// }

// func (s *MobiusWebhookService) handleUpdateSubscription(ctx context.Context) error {
// 	log.WithContext(ctx).
// 		WithField("eventType", s.Data.EventType).
// 		Info("Processing Mobius subscription update notification")

// 	mobiusPlanID := s.Data.EventBody.Plan.ID
// 	mobiusSubID := s.Data.EventBody.SubscriptionID

// 	if mobiusPlanID == "" {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing plan ID", map[string]interface{}{
// 			"subscription_id": mobiusSubID,
// 		}, nil)
// 	}

// 	if mobiusSubID == "" {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing subscription ID", map[string]interface{}{
// 			"plan_id": mobiusPlanID,
// 		}, nil)
// 	}

// 	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {

// 		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), mobiusSubID)
// 		if err != nil {
// 			if errors.Is(err, sql.ErrNoRows) {
// 				return fmt.Errorf("subscription not found for processor subscription ID: %s", mobiusSubID)
// 			}
// 			return fmt.Errorf("failed to get subscription: %w", err)
// 		}

// 		price, err := s.GetPriceByMobiusPlanID(ctx, mobiusPlanID)
// 		if err != nil {
// 			return fmt.Errorf("failed to find price for Mobius plan ID %s: %w", mobiusPlanID, err)
// 		}

// 		if err := sub.ActivateWithPrice(price); err != nil {
// 			return fmt.Errorf("failed to activate subscription: %w", err)
// 		}

// 		if err := sub.Validate(price.Amount); err != nil {
// 			return fmt.Errorf("failed to validate subscription: %w", err)
// 		}

// 		if err := s.UpdateSubscription(ctx, sub); err != nil {
// 			return fmt.Errorf("failed to update subscription: %w", err)
// 		}

// 		// grantParams := newGrantRoleParams(sub.UserID, sub.ID, models.ProcessorMobius, price, price.Product, db)
// 		// if err := grantRole(ctx, grantParams); err != nil {
// 		// 	return fmt.Errorf("failed to grant role: %w", err)
// 		// }
// 		// Role granting will be handled by transaction.sale.success webhook
// 		// TO DO: verify this behavior

// 		if s.BillingEventService != nil {
// 			metadata := map[string]interface{}{
// 				"processor_subscription_id": mobiusSubID,
// 				"plan_id":                   mobiusPlanID,
// 				"billing_cycle_days":        price.BillingCycleDays,
// 				"previous_period_end":       sub.CurrentPeriodEndsAt,
// 			}

// 			paymentEventData := billing.PaymentEventData{
// 				EventID:        uuid.New(),
// 				SubscriptionID: &sub.ID,
// 				UserID:         sub.UserID,
// 				EventType:      "charge_success",
// 				Processor:      "mobius",
// 				Amount:         &price.Amount,
// 				Currency:       price.Currency,
// 				WebhookSource:  "webhook",
// 				Metadata:       billing.CreateMetadataJSON(metadata),
// 			}

// 			if err := s.BillingEventService.LogPaymentEvent(ctx, paymentEventData); err != nil {
// 				log.WithError(err).Error("Failed to log payment event to ClickHouse")
// 			}
// 		}

// 		log.WithContext(ctx).WithFields(log.Fields{
// 			"subscriptionID": sub.ID,
// 			"userID":         sub.UserID,
// 			"priceID":        price.ID,
// 		}).Info("Updated subscription successfully")

// 		return nil
// 	})
// }

// func (s *MobiusWebhookService) handleDeleteSubscription(ctx context.Context) error {
// 	log.WithContext(ctx).
// 		WithField("eventType", s.Data.EventType).
// 		Info("Processing Mobius subscription delete notification")

// 	mobiusSubID := s.Data.EventBody.SubscriptionID

// 	if mobiusSubID == "" {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing subscription ID", map[string]interface{}{}, nil)
// 	}

// 	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {

// 		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), mobiusSubID)
// 		if err != nil {
// 			if errors.Is(err, sql.ErrNoRows) {
// 				return fmt.Errorf("subscription not found for processor subscription ID: %s", mobiusSubID)
// 			}
// 			return fmt.Errorf("failed to get subscription: %w", err)
// 		}

// 		cancelType := models.CancelTypeMerchant

// 		if err := sub.Cancel("Cancelled via Mobius webhook", &cancelType); err != nil {
// 			return fmt.Errorf("failed to cancel subscription: %w", err)
// 		}

// 		if err := s.UpdateSubscription(ctx, sub); err != nil {
// 			return fmt.Errorf("failed to update subscription: %w", err)
// 		}

// 		// Add notification to queue for user and send immediate email
// 		if s.NotificationService != nil {
// 			notification := &models.NotificationQueue{
// 				ID:        uuid.New(),
// 				UserID:    sub.UserID,
// 				EventType: models.NotificationPremiumEnded,
// 			}
// 			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
// 				log.WithContext(ctx).WithError(err).Error("failed to create and deliver membership ended notification")
// 			}
// 		}

// 		// Log subscription cancellation event to ClickHouse
// 		if s.BillingEventService != nil {
// 			metadata := map[string]interface{}{
// 				"processor_subscription_id": mobiusSubID,
// 				"cancel_type":               string(cancelType),
// 				"cancel_reason":             "Cancelled via Mobius webhook",
// 				"immediate_cancellation":    false,
// 			}

// 			subscriptionEventData := billing.SubscriptionEventData{
// 				EventID:                 uuid.New(),
// 				SubscriptionID:          sub.ID,
// 				UserID:                  sub.UserID,
// 				EventType:               "subscription_cancelled",
// 				Processor:               "mobius",
// 				ProcessorSubscriptionID: &mobiusSubID,
// 				Metadata:                billing.CreateMetadataJSON(metadata),
// 				Timestamp:               time.Now(),
// 			}

// 			if err := s.BillingEventService.LogSubscriptionEvent(ctx, subscriptionEventData); err != nil {
// 				log.WithError(err).Error("Failed to log subscription cancellation event to ClickHouse")
// 			}
// 		}

// 		log.WithContext(ctx).WithFields(log.Fields{
// 			"subscriptionID":          sub.ID,
// 			"userID":                  sub.UserID,
// 			"processorSubscriptionID": mobiusSubID,
// 		}).Info("Cancelled subscription successfully")

// 		return nil
// 	})
// }

// // handleTransactionSuccess processes successful transaction payments
// func (s *MobiusWebhookService) handleTransactionSuccess(ctx context.Context) error {
// 	log.WithContext(ctx).
// 		WithField("eventType", s.Data.EventType).
// 		Info("Processing Mobius transaction success notification")

// 	email := ""
// 	if s.Data.EventBody.BillingAddress != nil {
// 		email = s.Data.EventBody.BillingAddress.Email
// 	}
// 	transactionID := s.Data.EventBody.ProcessorID
// 	planID := s.Data.EventBody.Plan.ID
// 	amountStr := s.Data.EventBody.Plan.Amount

// 	if email == "" {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing email address", map[string]interface{}{
// 			"transaction_id": transactionID,
// 			"plan_id":        planID,
// 		}, nil)
// 	}

// 	if transactionID == "" {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing transaction ID", map[string]interface{}{
// 			"email":   email,
// 			"plan_id": planID,
// 		}, nil)
// 	}

// 	if planID == "" {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Missing plan ID", map[string]interface{}{
// 			"email":          email,
// 			"transaction_id": transactionID,
// 		}, nil)
// 	}

// 	// Parse amount
// 	amount, err := strconv.ParseFloat(amountStr, 64)
// 	if err != nil {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Failed to parse transaction amount", map[string]interface{}{
// 			"amount_string":  amountStr,
// 			"transaction_id": transactionID,
// 			"plan_id":        planID,
// 		}, err)
// 	}

// 	if amount <= 0 {
// 		return newMobiusBillingError(ErrorTypeMobiusValidation, "Invalid transaction amount", map[string]interface{}{
// 			"amount":         amount,
// 			"transaction_id": transactionID,
// 			"plan_id":        planID,
// 		}, nil)
// 	}

// 	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
// 		// 1. Check for duplicate transaction ID
// 		existingPurchase, err := s.GetPurchaseByTransactionID(ctx, models.ProcessorMobius, transactionID)
// 		if err == nil && existingPurchase != nil {
// 			log.WithContext(ctx).WithFields(log.Fields{
// 				"transactionID": transactionID,
// 				"existingID":    existingPurchase.ID,
// 			}).Info("Duplicate transaction detected, skipping processing")
// 			return nil // Idempotency - already processed
// 		}
// 		if err != nil && !errors.Is(err, sql.ErrNoRows) {
// 			return fmt.Errorf("failed to check for duplicate transaction: %w", err)
// 		}

// 		// 2. Find user by email
// 		user, err := s.GetGoTrueUserByEmail(ctx, email)
// 		if err != nil {
// 			return fmt.Errorf("failed to find user with email %s: %w", email, err)
// 		}

// 		// 3. Find price by Mobius plan ID
// 		price, err := s.GetPriceByMobiusPlanID(ctx, planID)
// 		if err != nil {
// 			return fmt.Errorf("failed to find price for Mobius plan ID %s: %w", planID, err)
// 		}

// 		// 4. Validate transaction amount matches expected price (with 2% tolerance like CCBill)
// 		expectedAmount := price.Amount
// 		tolerance := expectedAmount * 0.02
// 		if amount < (expectedAmount-tolerance) || amount > (expectedAmount+tolerance) {
// 			billingErr := newMobiusBillingError(ErrorTypeMobiusAmount,
// 				"Transaction amount does not match expected price",
// 				map[string]interface{}{
// 					"expected_amount": expectedAmount,
// 					"billed_amount":   amount,
// 					"tolerance":       tolerance,
// 					"price_id":        price.ID.String(),
// 					"plan_id":         planID,
// 				}, nil)

// 			s.logMobiusBillingError(ctx, billingErr, log.Fields{
// 				"transaction_id": transactionID,
// 				"email":          email,
// 			})
// 			return billingErr
// 		}

// 		// 5. Get product to determine role configuration
// 				product, err := s.GetProductByID(ctx, price.ProductID)
// 		if err != nil {
// 			return fmt.Errorf("failed to get product: %w", err)
// 		}

// 		var userRoleGrant *models.UserRoleGrant
// 		var extensionDays int

// 		// 5. Grant/extend role if product has role configured
// 		if product.RoleID != nil {

// 			// Determine extension days from product
// 			if product.RoleDurationDays != nil && *product.RoleDurationDays > 0 {
// 				extensionDays = *product.RoleDurationDays
// 			} else if price.BillingCycleDays != nil && *price.BillingCycleDays > 0 {
// 				extensionDays = *price.BillingCycleDays
// 			} else {
// 				extensionDays = 30 // Default fallback
// 			}

// 			// Extend the user's role expiration
// 			grant, _, err := s.ExtendRoleExpiration(ctx, user.ID, *product.RoleID, extensionDays)
// 			if err != nil {
// 				return fmt.Errorf("failed to extend role expiration: %w", err)
// 			}
// 			userRoleGrant = grant
// 		}

// 		// 6. Create Purchase record
// 		purchase := &models.Purchase{
// 			ID:              uuid.New(),
// 			UserID:          user.ID,
// 			PriceID:         price.ID,
// 			UserRoleGrantID: nil, // Set if role was granted
// 			Processor:       models.ProcessorMobius,
// 			TransactionID:   transactionID,
// 			Amount:          amount,
// 			Currency:        price.Currency,
// 			ExtensionDays:   nil, // Set if role was extended
// 			PurchasedAt:     time.Now(),
// 			CreatedAt:       time.Now(),
// 			UpdatedAt:       time.Now(),
// 		}

// 		// Set optional fields if role was granted
// 		if userRoleGrant != nil {
// 			purchase.UserRoleGrantID = &userRoleGrant.ID
// 			purchase.ExtensionDays = &extensionDays
// 		}

// 		if err := s.CreatePurchase(ctx, purchase); err != nil {
// 			return fmt.Errorf("failed to create purchase record: %w", err)
// 		}

// 		// 7. Log transaction success event to ClickHouse
// 		if s.BillingEventService != nil {
// 			metadata := map[string]interface{}{
// 				"transaction_id": transactionID,
// 				"plan_id":        planID,
// 				"product_id":     product.ID.String(),
// 				"role_granted":   product.RoleID != nil,
// 				"extension_days": extensionDays,
// 				"amount":         amount,
// 				"email":          email,
// 			}

// 			transactionEventData := billing.TransactionEventData{
// 				EventID:        uuid.New(),
// 				UserID:         &user.ID,
// 				SubscriptionID: nil, // Will be set if subscription-based
// 				EventType:      "payment_succeeded",
// 				Processor:      "mobius",
// 				TransactionID:  transactionID,
// 				Amount:         &amount,
// 				Currency:       price.Currency,
// 				Status:         "completed",
// 				Metadata:       billing.CreateMetadataJSON(metadata),
// 				Timestamp:      time.Now(),
// 			}

// 			if err := s.BillingEventService.LogTransactionEvent(ctx, transactionEventData); err != nil {
// 				log.WithError(err).Error("Failed to log transaction success event to ClickHouse")
// 			}
// 		}

// 		log.WithContext(ctx).WithFields(log.Fields{
// 			"userID":        user.ID,
// 			"transactionID": transactionID,
// 			"planID":        planID,
// 			"productID":     product.ID,
// 			"roleGranted":   product.RoleID != nil,
// 			"extensionDays": extensionDays,
// 			"purchaseID":    purchase.ID,
// 		}).Info("Successfully processed transaction success webhook")

// 		return nil
// 	})
// }

// // handleACUUpdated processes automatic card update notifications
// func (s *MobiusWebhookService) handleACUUpdated(ctx context.Context) error {
// 	// Extract customer/subscription info from webhook
// 	subscriptionID := s.Data.EventBody.SubscriptionID
// 	email := s.Data.EventBody.BillingAddress.Email
// 	vaultID := s.Data.EventBody.VaultID

// 	// Extract updated card details from webhook (these would come from actual Mobius webhook)
// 	var newLastFour, newCardType, newExpiryDate *string
// 	if cardInfo := s.Data.EventBody.PaymentMethod; cardInfo != nil {
// 		if cardInfo.LastFour != "" {
// 			newLastFour = &cardInfo.LastFour
// 		}
// 		if cardInfo.CardType != "" {
// 			newCardType = &cardInfo.CardType
// 		}
// 		if cardInfo.ExpiryDate != "" {
// 			newExpiryDate = &cardInfo.ExpiryDate
// 		}
// 	}

// 	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {

// 		// Find user by email
// 		user, err := s.GetGoTrueUserByEmail(ctx, email)
// 		if err != nil {
// 			log.WithError(err).Warn("Could not find user for ACU update webhook")
// 			return nil // Don't fail the webhook for missing user
// 		}

// 		// Find payment method by vault ID and processor
// 		paymentMethod, err := s.GetPaymentMethodByVaultID(ctx, models.ProcessorMobius, vaultID)
// 		if err != nil {
// 			log.WithError(err).Warn("Could not find payment method for ACU update webhook")
// 			return nil // Don't fail the webhook for missing payment method
// 		}

// 		// Verify payment method belongs to user
// 		if paymentMethod.UserID != user.ID {
// 			log.WithFields(log.Fields{
// 				"vaultID":           vaultID,
// 				"paymentMethodUser": paymentMethod.UserID,
// 				"webhookUser":       user.ID,
// 			}).Error("Payment method user mismatch in ACU update webhook")
// 			return nil
// 		}

// 		// Update payment method - ACU methods were removed since we don't track ACU status
// 		// Just mark as active since auto-update was successful
// 		paymentMethod.IsActive = true
// 		paymentMethod.FailureReason = nil

// 		if err := s.UpdatePaymentMethod(ctx, paymentMethod); err != nil {
// 			return fmt.Errorf("failed to update payment method after ACU update: %w", err)
// 		}

// 		// Log ACU update event to ClickHouse
// 		if s.BillingEventService != nil {
// 			metadata := map[string]interface{}{
// 				"subscription_id":      subscriptionID,
// 				"vault_id":             vaultID,
// 				"email":                email,
// 				"acu_status":           "automatically_updated",
// 				"card_updated":         true,
// 				"payment_method_id":    paymentMethod.ID.String(),
// 				"card_details_updated": newLastFour != nil || newCardType != nil || newExpiryDate != nil,
// 			}

// 			// Convert string subscription ID to UUID pointer
// 			var subID *uuid.UUID
// 			if subscriptionID != "" {
// 				if parsedID, err := uuid.Parse(subscriptionID); err == nil {
// 					subID = &parsedID
// 				}
// 			}

// 			acuEventData := billing.ACUEventData{
// 				EventID:        uuid.New(),
// 				SubscriptionID: subID,
// 				EventType:      "card_automatically_updated",
// 				Processor:      "mobius",
// 				UpdateStatus:   "success",
// 				RequiresAction: false,
// 				Metadata:       billing.CreateMetadataJSON(metadata),
// 				Timestamp:      time.Now(),
// 			}

// 			if err := s.BillingEventService.LogACUEvent(ctx, acuEventData); err != nil {
// 				log.WithError(err).Error("Failed to log ACU update event to ClickHouse")
// 			}
// 		}

// 		log.WithContext(ctx).WithFields(log.Fields{
// 			"userID":             user.ID,
// 			"subscriptionID":     subscriptionID,
// 			"vaultID":            vaultID,
// 			"paymentMethodID":    paymentMethod.ID,
// 			"cardDetailsUpdated": newLastFour != nil || newCardType != nil || newExpiryDate != nil,
// 		}).Info("Payment method automatically updated via ACU")

// 		return nil
// 	})
// }

// // handleACUContactCustomer processes ACU notifications requiring customer action
// func (s *MobiusWebhookService) handleACUContactCustomer(ctx context.Context) error {
// 	subscriptionID := s.Data.EventBody.SubscriptionID
// 	email := s.Data.EventBody.BillingAddress.Email

// 	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {

// 		// Find user and subscription
// 		user, err := s.GetGoTrueUserByEmail(ctx, email)
// 		if err != nil {
// 			log.WithError(err).Warn("Could not find user for ACU contact customer webhook")
// 			return nil // Don't fail the webhook for missing user
// 		}

// 		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), subscriptionID)
// 		if err != nil {
// 			log.WithError(err).Warn("Could not find subscription for ACU contact customer webhook")
// 			return nil // Don't fail the webhook for missing subscription
// 		}

// 		// Add notification to queue for user to update payment method
// 		if s.NotificationService != nil {
// 			notification := &models.NotificationQueue{
// 				ID:        uuid.New(),
// 				UserID:    user.ID,
// 				EventType: models.NotificationPaymentMethodUpdateRequired,
// 				Data: map[string]interface{}{
// 					"subscription_id": sub.ID.String(),
// 					"reason":          "Card update required by payment processor",
// 				},
// 			}

// 			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
// 				log.WithContext(ctx).WithError(err).Error("failed to create and deliver payment method update notification")
// 			}
// 		}

// 		// Log ACU contact customer event to ClickHouse
// 		if s.BillingEventService != nil {
// 			metadata := map[string]interface{}{
// 				"subscription_id":   subscriptionID,
// 				"email":             email,
// 				"acu_status":        "contact_customer",
// 				"requires_action":   true,
// 				"notification_sent": true,
// 			}

// 			// Convert string subscription ID to UUID pointer
// 			var subID *uuid.UUID
// 			if subscriptionID != "" {
// 				if parsedID, err := uuid.Parse(subscriptionID); err == nil {
// 					subID = &parsedID
// 				}
// 			}

// 			acuEventData := billing.ACUEventData{
// 				EventID:        uuid.New(),
// 				SubscriptionID: subID,
// 				EventType:      "card_update_required",
// 				Processor:      "mobius",
// 				UpdateStatus:   "pending",
// 				RequiresAction: true,
// 				Metadata:       billing.CreateMetadataJSON(metadata),
// 				Timestamp:      time.Now(),
// 			}

// 			if err := s.BillingEventService.LogACUEvent(ctx, acuEventData); err != nil {
// 				log.WithError(err).Error("Failed to log ACU contact customer event to ClickHouse")
// 			}
// 		}

// 		log.WithContext(ctx).WithFields(log.Fields{
// 			"userID":         user.ID,
// 			"subscriptionID": sub.ID,
// 			"email":          email,
// 		}).Info("Customer contact required for card update")

// 		return nil
// 	})
// }

// // handleACUClosedAccount processes notifications when customer's payment account is closed
// func (s *MobiusWebhookService) handleACUClosedAccount(ctx context.Context) error {
// 	subscriptionID := s.Data.EventBody.SubscriptionID
// 	email := s.Data.EventBody.BillingAddress.Email
// 	vaultID := s.Data.EventBody.VaultID

// 	return s.DB.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {

// 		// Find user and subscription
// 		user, err := s.GetGoTrueUserByEmail(ctx, email)
// 		if err != nil {
// 			log.WithError(err).Warn("Could not find user for ACU closed account webhook")
// 			return nil // Don't fail the webhook for missing user
// 		}

// 		sub, err := s.GetSubscriptionByProcessorSubscriptionID(ctx, string(models.ProcessorMobius), subscriptionID)
// 		if err != nil {
// 			log.WithError(err).Warn("Could not find subscription for ACU closed account webhook")
// 			return nil // Don't fail the webhook for missing subscription
// 		}

// 		// Find and mark payment method as inactive
// 		if vaultID != "" {
// 			paymentMethod, err := s.GetPaymentMethodByVaultID(ctx, models.ProcessorMobius, vaultID)
// 			if err != nil {
// 				log.WithError(err).Warn("Could not find payment method for ACU closed account webhook")
// 			} else if paymentMethod.UserID == user.ID {
// 				// Mark payment method as inactive due to closed account
// 				paymentMethod.MarkInactive("Payment account closed by bank")

// 				if err := s.UpdatePaymentMethod(ctx, paymentMethod); err != nil {
// 					log.WithError(err).Error("Failed to mark payment method as inactive after account closure")
// 				} else {
// 					log.WithContext(ctx).WithFields(log.Fields{
// 						"paymentMethodID": paymentMethod.ID,
// 						"vaultID":         vaultID,
// 					}).Info("Payment method marked inactive due to account closure")
// 				}
// 			}
// 		}

// 		// Put subscription in grace period or require new payment method
// 		now := time.Now()
// 		sub.Status = models.StatusPastDue
// 		sub.UpdatedAt = now

// 		if err := s.UpdateSubscription(ctx, sub); err != nil {
// 			return fmt.Errorf("failed to update subscription status: %w", err)
// 		}

// 		// Add urgent notification to user about closed account and send immediate email
// 		if s.NotificationService != nil {
// 			notification := &models.NotificationQueue{
// 				ID:        uuid.New(),
// 				UserID:    user.ID,
// 				EventType: models.NotificationPaymentMethodFailed,
// 				Data: map[string]interface{}{
// 					"subscription_id": sub.ID.String(),
// 					"reason":          "Payment account closed by bank",
// 					"urgency":         "high",
// 					"vault_id":        vaultID,
// 				},
// 			}

// 			if err := s.NotificationService.CreateAndDeliver(ctx, notification); err != nil {
// 				log.WithContext(ctx).WithError(err).Error("failed to create and deliver closed account notification")
// 			}
// 		}

// 		// Log ACU closed account event to ClickHouse
// 		if s.BillingEventService != nil {
// 			metadata := map[string]interface{}{
// 				"subscription_id":            subscriptionID,
// 				"vault_id":                   vaultID,
// 				"email":                      email,
// 				"acu_status":                 "closed_account",
// 				"account_closed":             true,
// 				"subscription_status":        string(sub.Status),
// 				"payment_method_deactivated": vaultID != "",
// 			}

// 			// Convert string subscription ID to UUID pointer
// 			var subID *uuid.UUID
// 			if subscriptionID != "" {
// 				if parsedID, err := uuid.Parse(subscriptionID); err == nil {
// 					subID = &parsedID
// 				}
// 			}

// 			acuEventData := billing.ACUEventData{
// 				EventID:        uuid.New(),
// 				SubscriptionID: subID,
// 				EventType:      "account_closed",
// 				Processor:      "mobius",
// 				UpdateStatus:   "failed",
// 				RequiresAction: true,
// 				Metadata:       billing.CreateMetadataJSON(metadata),
// 				Timestamp:      time.Now(),
// 			}

// 			if err := s.BillingEventService.LogACUEvent(ctx, acuEventData); err != nil {
// 				log.WithError(err).Error("Failed to log ACU closed account event to ClickHouse")
// 			}
// 		}

// 		log.WithContext(ctx).WithFields(log.Fields{
// 			"userID":         user.ID,
// 			"subscriptionID": sub.ID,
// 			"vaultID":        vaultID,
// 			"email":          email,
// 		}).Info("Payment account closed, subscription put on hold and payment method deactivated")

// 		return nil
// 	})
// }

// // handleChargebackComplete processes chargeback batch completion notifications
// func (s *MobiusWebhookService) handleChargebackComplete(ctx context.Context) error {
// 	// Chargeback events are typically batch operations containing multiple disputes
// 	// For now, we'll just log them for administrative purposes

// 	// Log chargeback batch completion to ClickHouse
// 	if s.BillingEventService != nil {
// 		metadata := map[string]interface{}{
// 			"batch_type":   "chargeback",
// 			"event_source": "mobius",
// 			"batch_status": "completed",
// 		}

// 		chargebackEventData := billing.ChargebackEventData{
// 			EventID:   uuid.New(),
// 			EventType: "batch_processed",
// 			Processor: "mobius",
// 			BatchID:   s.Data.EventID, // Use event ID as batch identifier
// 			Status:    "completed",
// 			Metadata:  billing.CreateMetadataJSON(metadata),
// 			Timestamp: time.Now(),
// 		}

// 		if err := s.BillingEventService.LogChargebackEvent(ctx, chargebackEventData); err != nil {
// 			log.WithError(err).Error("Failed to log chargeback batch event to ClickHouse")
// 		}
// 	}

// 	log.WithContext(ctx).WithFields(log.Fields{
// 		"eventID":   s.Data.EventID,
// 		"eventType": s.Data.EventType,
// 	}).Info("Chargeback batch processing completed")

// 	return nil
// }
