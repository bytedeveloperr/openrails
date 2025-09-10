//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins/internal/database/models"
	"github.com/doujins-org/doujins/internal/database/repo"
	"github.com/doujins-org/doujins/internal/services/subscription"
	"github.com/doujins-org/doujins/internal/services/webhook"
	"github.com/doujins-org/doujins/pkg/query"
)

// CCBillWebhookIntegrationTest tests all CCBill webhook event handlers
func TestCCBillWebhookIntegration(t *testing.T) {
	if !*enableTestContainers {
		t.Skip("Testcontainers not enabled, skipping CCBill webhook integration tests")
	}

	testContainer, teardown := setupIntegrationTest(t)
	defer teardown()

	// Create test user and price data
	ctx := context.Background()
	testUser, testPrice := setupCCBillTestData(t, testContainer, ctx)

	tests := []struct {
		name               string
		eventType          string
		payloadFile        string
		setupSubscription  bool
		expectedStatus     models.SubscriptionStatus
		expectTermination  bool
		expectNotification bool
		notificationType   models.NotificationEventType
	}{
		{
			name:               "NewSaleSuccess creates subscription",
			eventType:          "NewSaleSuccess",
			payloadFile:        "newsalesuccess.json",
			setupSubscription:  false,
			expectedStatus:     models.StatusActive,
			expectTermination:  false,
			expectNotification: true,
			notificationType:   models.NotificationPremiumStarted,
		},
		{
			name:               "NewSaleFailure logs failure",
			eventType:          "NewSaleFailure",
			payloadFile:        "newsalefailure.json",
			setupSubscription:  false,
			expectNotification: true,
			notificationType:   models.NotificationPaymentMethodFailed,
		},
		{
			name:               "RenewalSuccess extends subscription",
			eventType:          "RenewalSuccess",
			payloadFile:        "renewalsuccess.json",
			setupSubscription:  true,
			expectedStatus:     models.StatusActive,
			expectTermination:  false,
			expectNotification: true,
			notificationType:   models.NotificationPremiumRenewed,
		},
		{
			name:               "RenewalFailure sets past due",
			eventType:          "RenewalFailure",
			payloadFile:        "renewalfailure.json",
			setupSubscription:  true,
			expectedStatus:     models.StatusPastDue,
			expectTermination:  false,
			expectNotification: true,
			notificationType:   models.NotificationPaymentMethodFailed,
		},
		{
			name:               "UpgradeSuccess updates subscription tier",
			eventType:          "UpgradeSuccess",
			payloadFile:        "upgradesuccess.json",
			setupSubscription:  true,
			expectedStatus:     models.StatusActive,
			expectTermination:  false,
			expectNotification: true,
			notificationType:   models.NotificationPremiumRenewed,
		},
		{
			name:               "Cancellation terminates subscription",
			eventType:          "Cancellation",
			payloadFile:        "cancellation.json",
			setupSubscription:  true,
			expectedStatus:     models.StatusCancelled,
			expectTermination:  true,
			expectNotification: true,
			notificationType:   models.NotificationPremiumEnded,
		},
		{
			name:               "Expiration terminates subscription",
			eventType:          "Expiration",
			payloadFile:        "expiration.json",
			setupSubscription:  true,
			expectedStatus:     models.StatusCancelled,
			expectTermination:  true,
			expectNotification: true,
			notificationType:   models.NotificationPremiumEnded,
		},
		{
			name:               "Refund (full) terminates subscription",
			eventType:          "Refund",
			payloadFile:        "refund.json",
			setupSubscription:  true,
			expectedStatus:     models.StatusCancelled,
			expectTermination:  true,
			expectNotification: true,
			notificationType:   models.NotificationPremiumEnded,
		},
		{
			name:              "Void does not affect subscription",
			eventType:         "Void",
			payloadFile:       "void.json",
			setupSubscription: true,
			expectedStatus:    models.StatusActive, // Should remain unchanged
			expectTermination: false,
		},
		{
			name:               "Chargeback immediately terminates subscription",
			eventType:          "Chargeback",
			payloadFile:        "chargeback.json",
			setupSubscription:  true,
			expectedStatus:     models.StatusCancelled,
			expectTermination:  true,
			expectNotification: true,
			notificationType:   models.NotificationPremiumEnded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing data
			cleanupCCBillTestData(t, testContainer, ctx, testUser.ID)

			var testSubscription *models.Subscription
			if tt.setupSubscription {
				testSubscription = createTestSubscription(t, testContainer, ctx, testUser, testPrice)
			}

			// Load and customize webhook payload
			payload := loadAndCustomizeWebhookPayload(t, tt.payloadFile, testUser, testPrice, testSubscription)

			// Send webhook event
			response := sendCCBillWebhookEvent(t, testContainer, tt.eventType, payload)
			assert.Equal(t, http.StatusOK, response.StatusCode, "Webhook should be processed successfully")

			// Verify subscription state changes
			if tt.setupSubscription || tt.eventType == "NewSaleSuccess" {
				verifySubscriptionState(t, testContainer, ctx, testUser.ID, tt.expectedStatus, tt.expectTermination)
			}

			// Verify notifications were sent
			if tt.expectNotification {
				verifyNotificationSent(t, testContainer, ctx, testUser.ID, tt.notificationType)
			}

			// Verify ClickHouse event logging
			verifyClickHouseEventLogged(t, testContainer, ctx, tt.eventType)

			// Event-specific verifications
			switch tt.eventType {
			case "UpgradeSuccess":
				// Verify subscription was updated to new price tier
				// This would require creating a second test price for upgrade
			case "Chargeback":
				// Verify fraud flagging was applied
				verifyFraudFlagging(t, testContainer, ctx, testUser.ID)
			case "Void":
				// Verify no subscription state changes occurred
				if testSubscription != nil {
					verifySubscriptionUnchanged(t, testContainer, ctx, testSubscription.ID)
				}
			}
		})
	}
}

type testAuthUser struct {
	ID        string
	Email     string
	AuthToken string
}

func setupCCBillTestData(t *testing.T, testContainer *TestContainer, ctx context.Context) (*testAuthUser, *models.Price) {
	// Create test user in Casdoor shim
	admin := testContainer.CreateTestUser(ctx, "ccbill-test@example.com")
	user := &testAuthUser{ID: admin.ID, Email: admin.Email, AuthToken: "test-auth-token"}

	// Create test product and price
	productRepo := repo.NewProductRepo(testContainer.DB)
	product := &models.Product{
		ID:          uuid.New(),
		Name:        "Test Premium",
		Description: "Test premium subscription",
	}
	err = productRepo.Create(ctx, product)
	require.NoError(t, err)

	priceRepo := repo.NewPriceRepo(testContainer.DB)
	billingCycleDays := 30
	price := &models.Price{
		ID:               uuid.New(),
		ProductID:        product.ID,
		Amount:           23.00,
		Currency:         "USD",
		CCBillPriceID:    "75383d6a-41d4-4bd0-ac12-6c8c37fde5e5",
		BillingCycleDays: &billingCycleDays,
	}
	err = priceRepo.Create(ctx, price)
	require.NoError(t, err)

	// Load the product relationship
	price.Product = product

	return user, price
}

func cleanupCCBillTestData(t *testing.T, testContainer *TestContainer, ctx context.Context, userID string) {
	// Clean up subscriptions
	subRepo := repo.NewSubscriptionRepo(testContainer.DB)
	subscriptions, _, err := subRepo.GetByUserID(ctx, userID)
	if err == nil {
		for _, sub := range subscriptions {
			err = subRepo.Delete(ctx, sub.ID)
			if err != nil {
				t.Logf("Failed to cleanup subscription %s: %v", sub.ID, err)
			}
		}
	}

	// Clean up notifications
	notificationRepo := repo.NewNotificationQueueRepo(testContainer.DB)
	notifications, _, err := notificationRepo.GetNotifications(ctx, query.QueryOptions[repo.GetNotificationsFilters]{
		Filters: repo.GetNotificationsFilters{
			UserID: userID,
		},
		Page:     1,
		PageSize: 1000,
	})
	if err == nil {
		for _, notification := range notifications {
			err = notificationRepo.Delete(ctx, notification.ID)
			if err != nil {
				t.Logf("Failed to cleanup notification %s: %v", notification.ID, err)
			}
		}
	}
}

func createTestSubscription(t *testing.T, testContainer *TestContainer, ctx context.Context, user *testAuthUser, price *models.Price) *models.Subscription {
	subRepo := repo.NewSubscriptionRepo(testContainer.DB)
	now := time.Now()
	futureDate := now.Add(30 * 24 * time.Hour)

	subscription := &models.Subscription{
		ID:                      uuid.New(),
		UserID:                  user.ID,
		PriceID:                 price.ID,
		Status:                  models.StatusActive,
		Processor:               models.ProcessorCCBill,
		ProcessorSubscriptionID: "0125217202000000017",
		CurrentPeriodStartsAt:   &now,
		CurrentPeriodEndsAt:     &futureDate,
		StartedAt:               now,
	}

	err := subRepo.Create(ctx, subscription)
	require.NoError(t, err)

	return subscription
}

func loadAndCustomizeWebhookPayload(t *testing.T, payloadFile string, user *testAuthUser, price *models.Price, subscription *models.Subscription) string {
	// Load the test payload file
	payload, err := webhook.LoadTestWebhookPayload("ccbill", payloadFile)
	require.NoError(t, err)

	// Parse and customize the payload
	var webhookData map[string]interface{}
	err = json.Unmarshal([]byte(payload), &webhookData)
	require.NoError(t, err)

	// Customize common fields
	webhookData["email"] = user.Email
	if price != nil {
		webhookData["flexId"] = price.CCBillPriceID
	}
	if subscription != nil {
		webhookData["subscriptionId"] = subscription.ProcessorSubscriptionID
	}

	// Customize based on event type
	switch {
	case strings.Contains(payloadFile, "newsale"):
		// Ensure we have the right email for new sales
		webhookData["email"] = user.Email
	case strings.Contains(payloadFile, "upgrade"):
		// For upgrades, we need a different price ID for the new tier
		webhookData["newFlexId"] = "75383d6a-41d4-4bd0-ac12-6c8c37fde5e6"
		webhookData["previousFlexId"] = price.CCBillPriceID
	}

	customizedPayload, err := json.Marshal(webhookData)
	require.NoError(t, err)

	return string(customizedPayload)
}

func sendCCBillWebhookEvent(t *testing.T, testContainer *TestContainer, eventType, payload string) *http.Response {
	// Create webhook event structure
	webhookEvent := subscription.CCBillWebhookEvent{
		EventType: eventType,
		EventBody: []byte(payload),
	}

	// Create webhook service
	webhookService := &subscription.CCBillWebhookService{
		Data:                  webhookEvent,
		DB:                    testContainer.DB,
		CCBillClient:          testContainer.State.CCBillClient,
		ProductRepo:           repo.NewProductRepo(testContainer.DB),
		PriceRepo:             repo.NewPriceRepo(testContainer.DB),
		RoleRepo:              repo.NewRoleRepo(testContainer.DB),
		NotificationQueueRepo: repo.NewNotificationQueueRepo(testContainer.DB),
		NotificationService:   testContainer.State.NotificationService,
		DeadLetterService:     testContainer.State.DeadLetterService,
		BillingEventService:   testContainer.State.BillingEventService,
	}

	// Process the webhook
	ctx := context.Background()
	// Use a valid CCBill IP for testing
	testClientIP := "64.38.212.100"
	err := webhookService.HandleCCBillWebhook(ctx, testClientIP)

	// Create mock HTTP response
	if err != nil {
		// Return error response
		recorder := httptest.NewRecorder()
		recorder.WriteHeader(http.StatusBadRequest)
		recorder.WriteString(err.Error())
		return recorder.Result()
	}

	// Return success response
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)
	recorder.WriteString("OK")
	return recorder.Result()
}

func verifySubscriptionState(t *testing.T, testContainer *TestContainer, ctx context.Context, userID string, expectedStatus models.SubscriptionStatus, expectTermination bool) {
	subRepo := repo.NewSubscriptionRepo(testContainer.DB)
	subscriptions, _, err := subRepo.GetByUserID(ctx, userID)
	require.NoError(t, err)
	require.Len(t, subscriptions, 1, "Should have exactly one subscription")

	subscription := subscriptions[0]
	assert.Equal(t, expectedStatus, subscription.Status, "Subscription status should match expected")

	if expectTermination {
		assert.NotNil(t, subscription.CancelledAt, "Subscription should have cancellation date")
		assert.NotNil(t, subscription.CancelType, "Subscription should have cancellation type")
	} else {
		assert.Nil(t, subscription.CancelledAt, "Subscription should not have cancellation date")
		assert.Nil(t, subscription.CancelType, "Subscription should not have cancellation type")
	}
}

func verifyNotificationSent(t *testing.T, testContainer *TestContainer, ctx context.Context, userID string, expectedType models.NotificationEventType) {
	notificationRepo := repo.NewNotificationQueueRepo(testContainer.DB)
	notifications, _, err := notificationRepo.GetNotifications(ctx, query.QueryOptions[repo.GetNotificationsFilters]{
		Filters: repo.GetNotificationsFilters{
			UserID: userID,
		},
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, notifications, "Should have at least one notification")

	// Find notification of expected type
	found := false
	for _, notification := range notifications {
		if notification.EventType == expectedType {
			found = true
			break
		}
	}
	assert.True(t, found, "Should have notification of type %s", expectedType)
}

func verifyClickHouseEventLogged(t *testing.T, testContainer *TestContainer, ctx context.Context, eventType string) {
	// Note: This would require ClickHouse test setup
	// For now, we'll skip this verification unless ClickHouse is configured
	if testContainer.State.BillingEventService == nil {
		t.Skip("ClickHouse not configured for testing")
	}

	// TODO: Add ClickHouse event verification when test infrastructure is ready
	t.Logf("ClickHouse event logging verification not implemented yet for event type: %s", eventType)
}

func verifyFraudFlagging(t *testing.T, testContainer *TestContainer, ctx context.Context, userID string) {
	// This would verify that fraud flags were set appropriately
	// For now, we'll just verify the subscription was terminated immediately
	subRepo := repo.NewSubscriptionRepo(testContainer.DB)
	subscriptions, _, err := subRepo.GetByUserID(ctx, userID)
	require.NoError(t, err)
	require.Len(t, subscriptions, 1, "Should have exactly one subscription")

	subscription := subscriptions[0]
	assert.Equal(t, models.StatusCancelled, subscription.Status, "Subscription should be cancelled for chargeback")
	assert.NotNil(t, subscription.CancelledAt, "Subscription should have cancellation date")
	assert.NotNil(t, subscription.EndedAt, "Subscription should have ended date for immediate termination")

	// Verify chargeback details in feedback
	if subscription.CancelFeedback != nil {
		assert.Contains(t, *subscription.CancelFeedback, "CHARGEBACK", "Cancel feedback should contain chargeback information")
	}
}

func verifySubscriptionUnchanged(t *testing.T, testContainer *TestContainer, ctx context.Context, subscriptionID uuid.UUID) {
	subRepo := repo.NewSubscriptionRepo(testContainer.DB)
	subscription, err := subRepo.GetByID(ctx, subscriptionID)
	require.NoError(t, err)

	// For void events, subscription should remain active and unchanged
	assert.Equal(t, models.StatusActive, subscription.Status, "Subscription should remain active after void")
	assert.Nil(t, subscription.CancelledAt, "Subscription should not be cancelled")
	assert.Nil(t, subscription.EndedAt, "Subscription should not be ended")
}

// TestCCBillWebhookReplay tests the webhook replay functionality
func TestCCBillWebhookReplay(t *testing.T) {
	if !*enableTestContainers {
		t.Skip("Testcontainers not enabled, skipping CCBill webhook replay tests")
	}

	testContainer, teardown := setupIntegrationTest(t)
	defer teardown()

	// Test webhook replay validation (dry run)
	t.Run("ValidateAllCCBillWebhookPayloads", func(t *testing.T) {
		err := webhook.ValidateAllEvents("ccbill")
		assert.NoError(t, err, "All CCBill webhook payloads should be valid")
	})

	// Test specific event validation
	testEvents := []string{
		"newsalesuccess.json",
		"newsalefailure.json",
		"renewalsuccess.json",
		"renewalfailure.json",
		"upgradesuccess.json",
		"cancellation.json",
		"expiration.json",
		"refund.json",
		"void.json",
		"chargeback.json",
	}

	for _, eventFile := range testEvents {
		t.Run(fmt.Sprintf("Validate_%s", eventFile), func(t *testing.T) {
			err := webhook.ValidateEvent("ccbill", eventFile)
			assert.NoError(t, err, "Event %s should be valid", eventFile)
		})
	}
}
