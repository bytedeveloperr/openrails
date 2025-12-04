//go:build integration

package tests

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
)

// TestProduct represents a seeded product with its prices
type TestProduct struct {
	Product *models.Product
	Prices  []*models.Price
}

// CCBill test data constants - these match values in testdata/webhooks/ccbill/*.json
const (
	// CCBillTestFlexID is the flexId from saved webhook payloads
	CCBillTestFlexID = "75383d6a-41d4-4bd0-ac12-6c8c37fde5e5"
	// CCBillTestFormName is the formName from saved webhook payloads
	CCBillTestFormName = "211cc"
	// CCBillTestUsername is the username from saved newsalesuccess.json webhook
	CCBillTestUsername = "a9ab7b27-a31c-45cd-9bb5-8a38999afd7d"
	// CCBillTestUsername2 is the username from other webhook files (upgrade, reactivation, etc.)
	CCBillTestUsername2 = "test_user_8421"
	// CCBillTestUserID is the user ID we map the first test username to (must be a valid UUID)
	CCBillTestUserID = "cccccccc-cccc-cccc-cccc-cccccccc0001"
	// CCBillTestUserID2 is the user ID we map the second test username to (must be a valid UUID)
	// Note: ccbill_username_aliases has unique constraint on user_id, so each alias needs different user
	CCBillTestUserID2 = "cccccccc-cccc-cccc-cccc-cccccccc0002"
	// CCBillTestSubscriptionID is the subscriptionId from saved webhook payloads
	CCBillTestSubscriptionID = "0125217202000000017"
	// CCBillTestVoidSubscriptionID is from the void.json webhook (different subscription)
	CCBillTestVoidSubscriptionID = "0125217202000000020"
)

// DefaultTestProducts returns the standard set of test products
func (suite *TestContainerSuite) DefaultTestProducts() []TestProduct {
	return []TestProduct{
		{
			Product: &models.Product{
				ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				Slug:        "premium-monthly",
				DisplayName: "Premium Monthly",
				Description: "Monthly premium subscription",
				EntitlementsSpec: map[string]*int{
					"premium": nil, // Indefinite while subscription is active
				},
				IsActive: true,
			},
			Prices: []*models.Price{
				{
					ID:               uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					DisplayName:      "Monthly - $9.99",
					Amount:           999, // Amount in cents ($9.99)
					Currency:         "USD",
					BillingCycleDays: intPtr(30),
					NMIPlanID:        strPtr("plan_monthly_999"),
					NMIProvider:      strPtr("mobius"),
					// CCBillPriceID matches flexId from testdata/webhooks/ccbill/newsalesuccess.json
					CCBillPriceID: strPtr(CCBillTestFlexID),
					IsActive:      true,
				},
			},
		},
		{
			Product: &models.Product{
				ID:          uuid.MustParse("33333333-3333-3333-3333-333333333333"),
				Slug:        "premium-yearly",
				DisplayName: "Premium Yearly",
				Description: "Yearly premium subscription with discount",
				EntitlementsSpec: map[string]*int{
					"premium": nil,
				},
				IsActive: true,
			},
			Prices: []*models.Price{
				{
					ID:               uuid.MustParse("44444444-4444-4444-4444-444444444444"),
					DisplayName:      "Yearly - $99.99",
					Amount:           9999, // Amount in cents ($99.99)
					Currency:         "USD",
					BillingCycleDays: intPtr(365),
					NMIPlanID:        strPtr("plan_yearly_9999"),
					NMIProvider:      strPtr("mobius"),
					CCBillPriceID:    strPtr("ccbill_yearly_9999"),
					IsActive:         true,
				},
			},
		},
	}
}

// SeedProducts creates test products in the database (idempotent - uses UPSERT)
func (suite *TestContainerSuite) SeedProducts() []TestProduct {
	suite.t.Helper()
	ctx := context.Background()

	testProducts := suite.DefaultTestProducts()
	now := time.Now()

	for i := range testProducts {
		tp := &testProducts[i]
		tp.Product.CreatedAt = now
		tp.Product.UpdatedAt = now

		// Use ON CONFLICT to make this idempotent
		_, err := suite.BunDB.NewInsert().Model(tp.Product).
			On("CONFLICT (id) DO UPDATE").
			Set("display_name = EXCLUDED.display_name").
			Set("updated_at = EXCLUDED.updated_at").
			Exec(ctx)
		require.NoError(suite.t, err, "Failed to seed product %s", tp.Product.Slug)

		for _, price := range tp.Prices {
			price.ProductID = tp.Product.ID
			price.CreatedAt = now
			price.UpdatedAt = now

			_, err := suite.BunDB.NewInsert().Model(price).
				On("CONFLICT (id) DO UPDATE").
				Set("display_name = EXCLUDED.display_name").
				Set("updated_at = EXCLUDED.updated_at").
				Exec(ctx)
			require.NoError(suite.t, err, "Failed to seed price %s", price.DisplayName)
		}
	}

	return testProducts
}

// SeedProductsWithPrices is an alias for SeedProducts for clarity
func (suite *TestContainerSuite) SeedProductsWithPrices() []TestProduct {
	return suite.SeedProducts()
}

// GetSeededProduct retrieves a product by slug from the database
func (suite *TestContainerSuite) GetSeededProduct(slug string) *models.Product {
	suite.t.Helper()
	ctx := context.Background()

	var product models.Product
	err := suite.BunDB.NewSelect().
		Model(&product).
		Where("slug = ?", slug).
		Relation("Prices").
		Scan(ctx)
	require.NoError(suite.t, err, "Failed to get product %s", slug)

	return &product
}

// GetSeededPrice retrieves a price by ID from the database
func (suite *TestContainerSuite) GetSeededPrice(priceID uuid.UUID) *models.Price {
	suite.t.Helper()
	ctx := context.Background()

	var price models.Price
	err := suite.BunDB.NewSelect().
		Model(&price).
		Where("id = ?", priceID).
		Relation("Product").
		Scan(ctx)
	require.NoError(suite.t, err, "Failed to get price %s", priceID)

	return &price
}

// CreateTestSubscription creates a test subscription for a user
func (suite *TestContainerSuite) CreateTestSubscription(userID string, priceID uuid.UUID, status models.SubscriptionStatus) *models.Subscription {
	suite.t.Helper()
	ctx := context.Background()
	now := time.Now()

	periodStart := now
	periodEnd := now.Add(30 * 24 * time.Hour)

	sub := &models.Subscription{
		ID:                      uuid.New(),
		UserID:                  userID,
		PriceID:                 priceID,
		Status:                  status,
		StartedAt:               now,
		CurrentPeriodStartsAt:   &periodStart,
		CurrentPeriodEndsAt:     &periodEnd,
		Processor:               models.ProcessorNMI,
		ProcessorProvider:       strPtr("mobius"),
		ProcessorSubscriptionID: "test-sub-" + uuid.New().String()[:8],
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	_, err := suite.BunDB.NewInsert().Model(sub).Exec(ctx)
	require.NoError(suite.t, err, "Failed to create test subscription")

	return sub
}

// CreateTestSubscriptionWithOptions creates a subscription with custom options
type SubscriptionOptions struct {
	UserID            string
	PriceID           uuid.UUID
	Status            models.SubscriptionStatus
	Processor         models.Processor
	ProcessorProvider string
	PeriodStart       time.Time
	PeriodEnd         time.Time
	PaymentMethodID   *uuid.UUID
	CancelType        *models.CancelType
	CancelFeedback    *string
	ProcessorSubID    string
	RetryAttempts     *int
	NextRetryAt       *time.Time
}

func (suite *TestContainerSuite) CreateTestSubscriptionWithOptions(opts SubscriptionOptions) *models.Subscription {
	suite.t.Helper()
	ctx := context.Background()
	now := time.Now()

	if opts.PeriodStart.IsZero() {
		opts.PeriodStart = now
	}
	if opts.PeriodEnd.IsZero() {
		opts.PeriodEnd = now.Add(30 * 24 * time.Hour)
	}
	if opts.Processor == "" {
		opts.Processor = models.ProcessorNMI
	}
	if opts.ProcessorProvider == "" {
		opts.ProcessorProvider = "mobius"
	}
	if opts.ProcessorSubID == "" {
		opts.ProcessorSubID = "test-sub-" + uuid.New().String()[:8]
	}

	sub := &models.Subscription{
		ID:                      uuid.New(),
		UserID:                  opts.UserID,
		PriceID:                 opts.PriceID,
		Status:                  opts.Status,
		StartedAt:               opts.PeriodStart,
		CurrentPeriodStartsAt:   &opts.PeriodStart,
		CurrentPeriodEndsAt:     &opts.PeriodEnd,
		Processor:               opts.Processor,
		ProcessorProvider:       &opts.ProcessorProvider,
		ProcessorSubscriptionID: opts.ProcessorSubID,
		PaymentMethodID:         opts.PaymentMethodID,
		CancelType:              opts.CancelType,
		CancelFeedback:          opts.CancelFeedback,
		RetryAttempts:           opts.RetryAttempts,
		NextRetryAt:             opts.NextRetryAt,
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	if opts.Status == models.StatusCancelled {
		cancelledAt := now
		sub.CancelledAt = &cancelledAt
		sub.EndedAt = &cancelledAt
	}

	_, err := suite.BunDB.NewInsert().Model(sub).Exec(ctx)
	require.NoError(suite.t, err, "Failed to create test subscription with options")

	return sub
}

// CreateTestPaymentMethod creates a test payment method for a user
func (suite *TestContainerSuite) CreateTestPaymentMethod(userID string) *models.PaymentMethod {
	suite.t.Helper()
	ctx := context.Background()
	now := time.Now()

	pm := &models.PaymentMethod{
		ID:                   uuid.New(),
		UserID:               userID,
		Processor:            models.ProcessorNMI,
		Provider:             strPtr("mobius"),
		VaultID:              "vault-" + uuid.New().String()[:8],
		BillingID:            strPtr("billing-" + uuid.New().String()[:8]),
		InitialTransactionID: "txn-" + uuid.New().String()[:8],
		IsActive:             true,
		LastFour:             strPtr("4242"),
		CardType:             strPtr("Visa"),
		ExpiryDate:           strPtr("12/25"),
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	_, err := suite.BunDB.NewInsert().Model(pm).Exec(ctx)
	require.NoError(suite.t, err, "Failed to create test payment method")

	return pm
}

// CreateTestPaymentMethodWithOptions creates a payment method with custom options
type PaymentMethodOptions struct {
	UserID               string
	Processor            models.Processor
	Provider             string
	VaultID              string
	BillingID            string
	InitialTransactionID string
	IsActive             bool
	LastFour             string
	CardType             string
	ExpiryDate           string
	FailureReason        *string
}

func (suite *TestContainerSuite) CreateTestPaymentMethodWithOptions(opts PaymentMethodOptions) *models.PaymentMethod {
	suite.t.Helper()
	ctx := context.Background()
	now := time.Now()

	if opts.Processor == "" {
		opts.Processor = models.ProcessorNMI
	}
	if opts.Provider == "" {
		opts.Provider = "mobius"
	}
	if opts.VaultID == "" {
		opts.VaultID = "vault-" + uuid.New().String()[:8]
	}
	if opts.InitialTransactionID == "" {
		opts.InitialTransactionID = "txn-" + uuid.New().String()[:8]
	}

	pm := &models.PaymentMethod{
		ID:                   uuid.New(),
		UserID:               opts.UserID,
		Processor:            opts.Processor,
		Provider:             strPtr(opts.Provider),
		VaultID:              opts.VaultID,
		BillingID:            strPtrOrNil(opts.BillingID),
		InitialTransactionID: opts.InitialTransactionID,
		IsActive:             opts.IsActive,
		LastFour:             strPtrOrNil(opts.LastFour),
		CardType:             strPtrOrNil(opts.CardType),
		ExpiryDate:           strPtrOrNil(opts.ExpiryDate),
		FailureReason:        opts.FailureReason,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	_, err := suite.BunDB.NewInsert().Model(pm).Exec(ctx)
	require.NoError(suite.t, err, "Failed to create test payment method with options")

	return pm
}

// CreateTestPayment creates a test payment record
func (suite *TestContainerSuite) CreateTestPayment(userID string, priceID uuid.UUID, subscriptionID *uuid.UUID) *models.Payment {
	suite.t.Helper()
	ctx := context.Background()
	now := time.Now()

	payment := &models.Payment{
		ID:             uuid.New(),
		UserID:         userID,
		PriceID:        priceID,
		SubscriptionID: subscriptionID,
		Processor:      models.ProcessorNMI,
		TransactionID:  "txn-" + uuid.New().String()[:8],
		Amount:         999, // Amount in cents ($9.99)
		Currency:       "USD",
		PurchasedAt:    now,
		CreatedAt:      now,
	}

	_, err := suite.BunDB.NewInsert().Model(payment).Exec(ctx)
	require.NoError(suite.t, err, "Failed to create test payment")

	return payment
}

// CreateTestPaymentWithOptions creates a payment with custom options
type PaymentOptions struct {
	UserID            string
	PriceID           uuid.UUID
	SubscriptionID    *uuid.UUID
	RefundedPaymentID *uuid.UUID
	Processor         models.Processor
	ProcessorProvider string
	TransactionID     string
	Amount            int64 // Amount in cents
	Currency          string
	PurchasedAt       time.Time
}

func (suite *TestContainerSuite) CreateTestPaymentWithOptions(opts PaymentOptions) *models.Payment {
	suite.t.Helper()
	ctx := context.Background()
	now := time.Now()

	if opts.Processor == "" {
		opts.Processor = models.ProcessorNMI
	}
	if opts.TransactionID == "" {
		opts.TransactionID = "txn-" + uuid.New().String()[:8]
	}
	if opts.Amount == 0 {
		opts.Amount = 999 // Default: $9.99 in cents
	}
	if opts.Currency == "" {
		opts.Currency = "USD"
	}
	if opts.PurchasedAt.IsZero() {
		opts.PurchasedAt = now
	}

	payment := &models.Payment{
		ID:                uuid.New(),
		UserID:            opts.UserID,
		PriceID:           opts.PriceID,
		SubscriptionID:    opts.SubscriptionID,
		RefundedPaymentID: opts.RefundedPaymentID,
		Processor:         opts.Processor,
		ProcessorProvider: strPtrOrNil(opts.ProcessorProvider),
		TransactionID:     opts.TransactionID,
		Amount:            opts.Amount,
		Currency:          opts.Currency,
		PurchasedAt:       opts.PurchasedAt,
		CreatedAt:         now,
	}

	_, err := suite.BunDB.NewInsert().Model(payment).Exec(ctx)
	require.NoError(suite.t, err, "Failed to create test payment with options")

	return payment
}

// CreateTestEntitlement creates a test entitlement for a user
func (suite *TestContainerSuite) CreateTestEntitlement(userID string, entitlementName string, sourceID *uuid.UUID, sourceType models.EntitlementSourceType) *models.Entitlement {
	suite.t.Helper()
	ctx := context.Background()
	now := time.Now()

	// For subscription-sourced entitlements, end_at should be NULL (indefinite while subscription is active)
	// For other sources, we may want a finite window
	var endAt *time.Time
	if sourceType != models.EntitlementSourceSubscription {
		end := now.Add(30 * 24 * time.Hour)
		endAt = &end
	}

	ent := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: entitlementName,
		StartAt:     now,
		EndAt:       endAt,
		SourceID:    sourceID,
		SourceType:  sourceType,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := suite.BunDB.NewInsert().Model(ent).Exec(ctx)
	require.NoError(suite.t, err, "Failed to create test entitlement")

	return ent
}

// CreateTestNotification creates a test notification for a user
func (suite *TestContainerSuite) CreateTestNotification(userID string, eventType models.NotificationEventType, data map[string]any) *models.NotificationQueue {
	suite.t.Helper()
	ctx := context.Background()
	now := time.Now()

	notif := &models.NotificationQueue{
		ID:        uuid.New(),
		UserID:    userID,
		EventType: eventType,
		Data:      data,
		Seen:      false,
		CreatedAt: now,
	}

	_, err := suite.BunDB.NewInsert().Model(notif).Exec(ctx)
	require.NoError(suite.t, err, "Failed to create test notification")

	return notif
}

// Query helpers for assertions

// GetSubscription retrieves a subscription by ID
func (suite *TestContainerSuite) GetSubscription(id uuid.UUID) *models.Subscription {
	suite.t.Helper()
	ctx := context.Background()

	var sub models.Subscription
	err := suite.BunDB.NewSelect().
		Model(&sub).
		Where("sub.id = ?", id).
		Relation("Price").
		Relation("PaymentMethod").
		Scan(ctx)
	require.NoError(suite.t, err, "Failed to get subscription %s", id)

	return &sub
}

// GetSubscriptionByUserID retrieves the active subscription for a user
func (suite *TestContainerSuite) GetSubscriptionByUserID(userID string) *models.Subscription {
	suite.t.Helper()
	ctx := context.Background()

	var sub models.Subscription
	err := suite.BunDB.NewSelect().
		Model(&sub).
		Where("user_id = ?", userID).
		Where("status = ?", models.StatusActive).
		Relation("Price").
		Scan(ctx)
	if err != nil {
		return nil
	}

	return &sub
}

// GetAllSubscriptionsByUserID retrieves all subscriptions for a user
func (suite *TestContainerSuite) GetAllSubscriptionsByUserID(userID string) []*models.Subscription {
	suite.t.Helper()
	ctx := context.Background()

	var subs []*models.Subscription
	err := suite.BunDB.NewSelect().
		Model(&subs).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Relation("Price").
		Scan(ctx)
	require.NoError(suite.t, err, "Failed to get subscriptions for user %s", userID)

	return subs
}

// GetPaymentsByUserID retrieves all payments for a user
func (suite *TestContainerSuite) GetPaymentsByUserID(userID string) []*models.Payment {
	suite.t.Helper()
	ctx := context.Background()

	var payments []*models.Payment
	err := suite.BunDB.NewSelect().
		Model(&payments).
		Where("user_id = ?", userID).
		Order("purchased_at DESC").
		Relation("Price").
		Scan(ctx)
	require.NoError(suite.t, err, "Failed to get payments for user %s", userID)

	return payments
}

// GetPaymentMethodsByUserID retrieves all payment methods for a user
func (suite *TestContainerSuite) GetPaymentMethodsByUserID(userID string) []*models.PaymentMethod {
	suite.t.Helper()
	ctx := context.Background()

	var pms []*models.PaymentMethod
	err := suite.BunDB.NewSelect().
		Model(&pms).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Scan(ctx)
	require.NoError(suite.t, err, "Failed to get payment methods for user %s", userID)

	return pms
}

// GetEntitlementsByUserID retrieves all active entitlements for a user
func (suite *TestContainerSuite) GetEntitlementsByUserID(userID string) []*models.Entitlement {
	suite.t.Helper()
	ctx := context.Background()
	now := time.Now()

	var ents []*models.Entitlement
	err := suite.BunDB.NewSelect().
		Model(&ents).
		Where("user_id = ?", userID).
		Where("start_at <= ?", now).
		Where("(end_at IS NULL OR end_at > ?)", now).
		Where("revoked_at IS NULL").
		Where("deleted_at IS NULL").
		Scan(ctx)
	require.NoError(suite.t, err, "Failed to get entitlements for user %s", userID)

	return ents
}

// GetNotificationsByUserID retrieves all notifications for a user
func (suite *TestContainerSuite) GetNotificationsByUserID(userID string) []*models.NotificationQueue {
	suite.t.Helper()
	ctx := context.Background()

	var notifs []*models.NotificationQueue
	err := suite.BunDB.NewSelect().
		Model(&notifs).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Scan(ctx)
	require.NoError(suite.t, err, "Failed to get notifications for user %s", userID)

	return notifs
}

// CountUnreadNotifications returns the count of unread notifications for a user
func (suite *TestContainerSuite) CountUnreadNotifications(userID string) int {
	suite.t.Helper()
	ctx := context.Background()

	count, err := suite.BunDB.NewSelect().
		Model((*models.NotificationQueue)(nil)).
		Where("user_id = ?", userID).
		Where("seen = ?", false).
		Count(ctx)
	require.NoError(suite.t, err, "Failed to count unread notifications for user %s", userID)

	return count
}

// GetWebhookEventsByProcessor retrieves all webhook events for a processor
func (suite *TestContainerSuite) GetWebhookEventsByProcessor(processor string) []*models.WebhookEvent {
	suite.t.Helper()
	ctx := context.Background()

	var events []*models.WebhookEvent
	err := suite.BunDB.NewSelect().
		Model(&events).
		Where("processor = ?", processor).
		Order("created_at DESC").
		Scan(ctx)
	require.NoError(suite.t, err, "Failed to get webhook events for processor %s", processor)

	return events
}

// GetWebhookEventByEventType retrieves the most recent webhook event by type
func (suite *TestContainerSuite) GetWebhookEventByEventType(processor, eventType string) *models.WebhookEvent {
	suite.t.Helper()
	ctx := context.Background()

	var event models.WebhookEvent
	err := suite.BunDB.NewSelect().
		Model(&event).
		Where("processor = ?", processor).
		Where("event_type = ?", eventType).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil
	}

	return &event
}

// CountWebhookEvents returns the count of webhook events for a processor
func (suite *TestContainerSuite) CountWebhookEvents(processor string) int {
	suite.t.Helper()
	ctx := context.Background()

	count, err := suite.BunDB.NewSelect().
		Model((*models.WebhookEvent)(nil)).
		Where("processor = ?", processor).
		Count(ctx)
	require.NoError(suite.t, err, "Failed to count webhook events for processor %s", processor)

	return count
}

// WaitForWebhookProcessed waits for a webhook event to be processed (or fail)
// Returns the webhook event status and any error
func (suite *TestContainerSuite) WaitForWebhookProcessed(eventID uuid.UUID, timeout time.Duration) (string, error) {
	suite.t.Helper()
	ctx := context.Background()

	deadline := time.Now().Add(timeout)
	pollInterval := 100 * time.Millisecond

	for time.Now().Before(deadline) {
		var event models.WebhookEvent
		err := suite.BunDB.NewSelect().
			Model(&event).
			Where("id = ?", eventID).
			Scan(ctx)
		if err != nil {
			return "", err
		}

		// Check if processing is complete (processed, failed, or duplicate)
		if event.Status != "pending" {
			return event.Status, nil
		}

		time.Sleep(pollInterval)
	}

	return "", context.DeadlineExceeded
}

// WaitForLatestWebhookProcessed waits for the most recent webhook event of a type to be processed
func (suite *TestContainerSuite) WaitForLatestWebhookProcessed(processor, eventType string, timeout time.Duration) (*models.WebhookEvent, error) {
	suite.t.Helper()
	ctx := context.Background()

	deadline := time.Now().Add(timeout)
	pollInterval := 100 * time.Millisecond

	for time.Now().Before(deadline) {
		var event models.WebhookEvent
		err := suite.BunDB.NewSelect().
			Model(&event).
			Where("processor = ?", processor).
			Where("event_type = ?", eventType).
			Order("created_at DESC").
			Limit(1).
			Scan(ctx)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		// Check if processing is complete (final states: processed, failed, duplicate)
		// Note: "pending" and "processing" are intermediate states
		if event.Status == "processed" || event.Status == "failed" || event.Status == "duplicate" {
			return &event, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, context.DeadlineExceeded
}

// GetSubscriptionByProcessorID retrieves a subscription by processor subscription ID
func (suite *TestContainerSuite) GetSubscriptionByProcessorID(processorSubID string) *models.Subscription {
	suite.t.Helper()
	ctx := context.Background()

	var sub models.Subscription
	err := suite.BunDB.NewSelect().
		Model(&sub).
		Where("processor_subscription_id = ?", processorSubID).
		Relation("Price").
		Scan(ctx)
	if err != nil {
		return nil
	}

	return &sub
}

// CreateCCBillAlias creates a CCBill username alias mapping for testing
// This simulates what happens when a user starts the FlexForm checkout flow
func (suite *TestContainerSuite) CreateCCBillAlias(alias string, userID string) *models.CCBillUsernameAlias {
	suite.t.Helper()
	ctx := context.Background()
	now := time.Now()

	aliasModel := &models.CCBillUsernameAlias{
		Alias:     alias,
		UserID:    userID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := suite.BunDB.NewInsert().Model(aliasModel).
		On("CONFLICT (alias) DO UPDATE").
		Set("user_id = EXCLUDED.user_id").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	require.NoError(suite.t, err, "Failed to create CCBill alias")

	return aliasModel
}

// SeedCCBillTestData seeds all data needed for CCBill webhook replay tests
// This creates the alias mappings that connect webhook payloads to test users
// Note: Each alias maps to a different user because ccbill_username_aliases has
// a unique constraint on user_id (one alias per user)
func (suite *TestContainerSuite) SeedCCBillTestData() {
	suite.t.Helper()

	// Create CCBill alias mappings (usernames from webhooks → test user IDs)
	// CCBillTestUsername is used in newsalesuccess.json
	suite.CreateCCBillAlias(CCBillTestUsername, CCBillTestUserID)
	// CCBillTestUsername2 is used in other webhooks (upgrade, reactivation, billingdatechange, etc.)
	suite.CreateCCBillAlias(CCBillTestUsername2, CCBillTestUserID2)
}

// SeedCCBillTestDataWithSubscription seeds CCBill test data including an active subscription
// This is needed for tests that require an existing subscription (renewal, cancellation, etc.)
func (suite *TestContainerSuite) SeedCCBillTestDataWithSubscription() *models.Subscription {
	suite.t.Helper()

	// First seed the basic test data
	suite.SeedCCBillTestData()

	// Get the monthly price (which has the CCBillTestFlexID)
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create an active subscription with the processor subscription ID from saved webhooks
	return suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         CCBillTestUserID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: CCBillTestSubscriptionID,
	})
}

// CleanupSubscriptionsForUser deletes all subscriptions for a user
// Use this for test isolation when tests share the same suite
func (suite *TestContainerSuite) CleanupSubscriptionsForUser(userID string) {
	suite.t.Helper()
	ctx := context.Background()

	// Also delete entitlements for this user
	_, _ = suite.BunDB.NewDelete().
		Model((*models.Entitlement)(nil)).
		Where("user_id = ?", userID).
		Exec(ctx)

	// Delete subscriptions
	_, err := suite.BunDB.NewDelete().
		Model((*models.Subscription)(nil)).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		suite.t.Logf("Warning: failed to cleanup subscriptions for user %s: %v", userID, err)
	}
}

// Helper functions for pointers
func intPtr(i int) *int {
	return &i
}

func strPtr(s string) *string {
	return &s
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
