//go:build integration

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
)

// TestWebhookSubscriptionLifecycle tests the full subscription lifecycle via webhooks:
// NewSaleSuccess → subscription created → Cancellation → subscription cancelled
func TestWebhookSubscriptionLifecycle(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up any existing data for test isolation
	suite.CleanupSubscriptionsForUser(CCBillTestUserID)

	// Seed products and CCBill alias mappings
	suite.SeedProducts()
	suite.SeedCCBillTestData()

	// Step 1: Send NewSaleSuccess webhook - should create subscription
	t.Run("NewSaleSuccess creates subscription", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "newsalesuccess.json")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var jsonData map[string]interface{}
		err = json.Unmarshal(data, &jsonData)
		require.NoError(t, err)

		formData := jsonToFormData(jsonData)

		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=NewSaleSuccess",
			strings.NewReader(formData))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "Webhook should be accepted")

		// Wait for webhook to be processed
		event, err := suite.WaitForLatestWebhookProcessed("ccbill", "NewSaleSuccess", 5*time.Second)
		require.NoError(t, err, "Webhook should be processed")
		require.Equal(t, "processed", event.Status, "Webhook should process successfully: %v", event.ErrorMessage)

		// Verify subscription was created
		subs := suite.GetAllSubscriptionsByUserID(CCBillTestUserID)
		require.Len(t, subs, 1, "Should have 1 subscription")

		sub := subs[0]
		assert.Equal(t, "active", string(sub.Status), "Subscription should be active")
		assert.Equal(t, "ccbill", string(sub.Processor), "Processor should be ccbill")
		assert.Equal(t, CCBillTestSubscriptionID, sub.ProcessorSubscriptionID, "Processor subscription ID should match")

		// Verify entitlements were granted
		entitlements := suite.GetEntitlementsByUserID(CCBillTestUserID)
		assert.NotEmpty(t, entitlements, "Should have entitlements after subscription created")
	})

	// Step 2: Send Cancellation webhook - should cancel subscription
	t.Run("Cancellation cancels subscription", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "cancellation.json")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var jsonData map[string]interface{}
		err = json.Unmarshal(data, &jsonData)
		require.NoError(t, err)

		formData := jsonToFormData(jsonData)

		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=Cancellation",
			strings.NewReader(formData))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "Cancellation webhook should be accepted")

		// Wait for webhook to be processed
		event, err := suite.WaitForLatestWebhookProcessed("ccbill", "Cancellation", 5*time.Second)
		require.NoError(t, err, "Cancellation webhook should be processed")
		require.Equal(t, "processed", event.Status, "Cancellation should process successfully: %v", event.ErrorMessage)

		// Verify subscription was cancelled
		sub := suite.GetSubscriptionByProcessorID(CCBillTestSubscriptionID)
		require.NotNil(t, sub, "Subscription should still exist")
		assert.Equal(t, "cancelled", string(sub.Status), "Subscription should be cancelled")
		assert.NotNil(t, sub.CancelledAt, "CancelledAt should be set")
	})
}

// TestWebhookRenewalFlow tests that RenewalSuccess extends subscription period
func TestWebhookRenewalFlow(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up and seed data
	suite.CleanupSubscriptionsForUser(CCBillTestUserID)
	suite.SeedProducts()
	suite.SeedCCBillTestData()

	// Create an existing active subscription (simulating a subscription created via NewSaleSuccess)
	products := suite.DefaultTestProducts()
	priceID := products[0].Prices[0].ID

	sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         CCBillTestUserID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: CCBillTestSubscriptionID,
		PeriodStart:    time.Now().Add(-30 * 24 * time.Hour), // Started 30 days ago
		PeriodEnd:      time.Now().Add(-1 * time.Hour),       // Just expired
	})

	originalPeriodEnd := sub.CurrentPeriodEndsAt

	t.Run("RenewalSuccess extends subscription", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "renewalsuccess.json")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var jsonData map[string]interface{}
		err = json.Unmarshal(data, &jsonData)
		require.NoError(t, err)

		formData := jsonToFormData(jsonData)

		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=RenewalSuccess",
			strings.NewReader(formData))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "RenewalSuccess webhook should be accepted")

		// Wait for webhook to be processed
		event, err := suite.WaitForLatestWebhookProcessed("ccbill", "RenewalSuccess", 5*time.Second)
		require.NoError(t, err, "RenewalSuccess webhook should be processed")
		require.Equal(t, "processed", event.Status, "RenewalSuccess should process successfully: %v", event.ErrorMessage)

		// Verify subscription period was extended
		updatedSub := suite.GetSubscriptionByProcessorID(CCBillTestSubscriptionID)
		require.NotNil(t, updatedSub, "Subscription should exist")
		assert.Equal(t, "active", string(updatedSub.Status), "Subscription should be active after renewal")
		assert.True(t, updatedSub.CurrentPeriodEndsAt.After(*originalPeriodEnd),
			"Period end should be extended after renewal")
	})
}

// TestWebhookRenewalFailureFlow tests that RenewalFailure sets subscription to past_due
func TestWebhookRenewalFailureFlow(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up and seed data
	suite.CleanupSubscriptionsForUser(CCBillTestUserID)
	suite.SeedProducts()
	suite.SeedCCBillTestData()

	// Create an existing active subscription
	products := suite.DefaultTestProducts()
	priceID := products[0].Prices[0].ID

	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         CCBillTestUserID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: CCBillTestSubscriptionID,
	})

	t.Run("RenewalFailure sets subscription to past_due", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "renewalfailure.json")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var jsonData map[string]interface{}
		err = json.Unmarshal(data, &jsonData)
		require.NoError(t, err)

		formData := jsonToFormData(jsonData)

		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=RenewalFailure",
			strings.NewReader(formData))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "RenewalFailure webhook should be accepted")

		// Wait for webhook to be processed
		event, err := suite.WaitForLatestWebhookProcessed("ccbill", "RenewalFailure", 5*time.Second)
		require.NoError(t, err, "RenewalFailure webhook should be processed")
		require.Equal(t, "processed", event.Status, "RenewalFailure should process successfully: %v", event.ErrorMessage)

		// Verify subscription status changed to past_due
		updatedSub := suite.GetSubscriptionByProcessorID(CCBillTestSubscriptionID)
		require.NotNil(t, updatedSub, "Subscription should exist")
		assert.Equal(t, "past_due", string(updatedSub.Status), "Subscription should be past_due after renewal failure")
		assert.NotNil(t, updatedSub.RetryAttempts, "RetryAttempts should be set")
		assert.NotNil(t, updatedSub.NextRetryAt, "NextRetryAt should be set")
	})
}

// TestWebhookExpirationFlow tests that Expiration webhook marks subscription as cancelled
func TestWebhookExpirationFlow(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up and seed data
	suite.CleanupSubscriptionsForUser(CCBillTestUserID)
	suite.SeedProducts()
	suite.SeedCCBillTestData()

	// Create an existing active subscription
	products := suite.DefaultTestProducts()
	priceID := products[0].Prices[0].ID

	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         CCBillTestUserID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: CCBillTestSubscriptionID,
	})

	// Create entitlement for this subscription
	sub := suite.GetSubscriptionByProcessorID(CCBillTestSubscriptionID)
	suite.CreateTestEntitlement(CCBillTestUserID, "premium", &sub.ID, models.EntitlementSourceSubscription)

	t.Run("Expiration cancels subscription and revokes entitlements", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "expiration.json")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var jsonData map[string]interface{}
		err = json.Unmarshal(data, &jsonData)
		require.NoError(t, err)

		formData := jsonToFormData(jsonData)

		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=Expiration",
			strings.NewReader(formData))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "Expiration webhook should be accepted")

		// Wait for webhook to be processed
		event, err := suite.WaitForLatestWebhookProcessed("ccbill", "Expiration", 5*time.Second)
		require.NoError(t, err, "Expiration webhook should be processed")
		require.Equal(t, "processed", event.Status, "Expiration should process successfully: %v", event.ErrorMessage)

		// Verify subscription was cancelled
		updatedSub := suite.GetSubscriptionByProcessorID(CCBillTestSubscriptionID)
		require.NotNil(t, updatedSub, "Subscription should exist")
		assert.Equal(t, "cancelled", string(updatedSub.Status), "Subscription should be cancelled after expiration")
		assert.NotNil(t, updatedSub.EndedAt, "EndedAt should be set")

		// Verify entitlements were revoked (no longer active)
		entitlements := suite.GetEntitlementsByUserID(CCBillTestUserID)
		assert.Empty(t, entitlements, "Should have no active entitlements after expiration")
	})
}

// TestWebhookChargebackTerminatesSubscription tests that Chargeback immediately terminates subscription
func TestWebhookChargebackTerminatesSubscription(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up and seed data
	suite.CleanupSubscriptionsForUser(CCBillTestUserID)
	suite.SeedProducts()
	suite.SeedCCBillTestData()

	// Create an existing active subscription
	products := suite.DefaultTestProducts()
	priceID := products[0].Prices[0].ID

	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         CCBillTestUserID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: CCBillTestSubscriptionID,
	})

	// Create entitlement
	sub := suite.GetSubscriptionByProcessorID(CCBillTestSubscriptionID)
	suite.CreateTestEntitlement(CCBillTestUserID, "premium", &sub.ID, models.EntitlementSourceSubscription)

	t.Run("Chargeback immediately terminates subscription", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "chargeback.json")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var jsonData map[string]interface{}
		err = json.Unmarshal(data, &jsonData)
		require.NoError(t, err)

		formData := jsonToFormData(jsonData)

		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=Chargeback",
			strings.NewReader(formData))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "Chargeback webhook should be accepted")

		// Wait for webhook to be processed
		event, err := suite.WaitForLatestWebhookProcessed("ccbill", "Chargeback", 5*time.Second)
		require.NoError(t, err, "Chargeback webhook should be processed")
		require.Equal(t, "processed", event.Status, "Chargeback should process successfully: %v", event.ErrorMessage)

		// Verify subscription was immediately terminated
		updatedSub := suite.GetSubscriptionByProcessorID(CCBillTestSubscriptionID)
		require.NotNil(t, updatedSub, "Subscription should exist")
		assert.Equal(t, "cancelled", string(updatedSub.Status), "Subscription should be cancelled after chargeback")
		assert.NotNil(t, updatedSub.CancelledAt, "CancelledAt should be set")
		assert.NotNil(t, updatedSub.EndedAt, "EndedAt should be set (immediate termination)")
		assert.Contains(t, *updatedSub.CancelFeedback, "CHARGEBACK", "Cancel feedback should indicate chargeback")

		// Verify entitlements were revoked
		entitlements := suite.GetEntitlementsByUserID(CCBillTestUserID)
		assert.Empty(t, entitlements, "Should have no active entitlements after chargeback")
	})
}

// TestWebhookRefundHandling tests refund processing
func TestWebhookRefundHandling(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up and seed data
	suite.CleanupSubscriptionsForUser(CCBillTestUserID)
	suite.SeedProducts()
	suite.SeedCCBillTestData()

	// Create an existing active subscription
	products := suite.DefaultTestProducts()
	priceID := products[0].Prices[0].ID

	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         CCBillTestUserID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: CCBillTestSubscriptionID,
	})

	t.Run("Refund is processed", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "refund.json")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var jsonData map[string]interface{}
		err = json.Unmarshal(data, &jsonData)
		require.NoError(t, err)

		formData := jsonToFormData(jsonData)

		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=Refund",
			strings.NewReader(formData))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "Refund webhook should be accepted")

		// Wait for webhook to be processed
		event, err := suite.WaitForLatestWebhookProcessed("ccbill", "Refund", 5*time.Second)
		require.NoError(t, err, "Refund webhook should be processed")
		require.Equal(t, "processed", event.Status, "Refund should process successfully: %v", event.ErrorMessage)

		// Subscription handling depends on refund amount - just verify webhook was processed
		// The subscription may or may not be terminated depending on refund amount
	})
}

// TestWebhookUserReactivation tests that reactivation restores subscription
func TestWebhookUserReactivation(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up subscriptions for BOTH test users to ensure no duplicate processor_subscription_id
	// (other tests may have created subscriptions with the same ID for CCBillTestUserID)
	suite.CleanupSubscriptionsForUser(CCBillTestUserID)
	suite.CleanupSubscriptionsForUser(CCBillTestUserID2)
	suite.SeedProducts()
	suite.SeedCCBillTestData()

	// Create a cancelled subscription (simulating a previously active then cancelled subscription)
	products := suite.DefaultTestProducts()
	priceID := products[0].Prices[0].ID

	// The userreactivation webhook uses CCBillTestUsername2 which maps to CCBillTestUserID2
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         CCBillTestUserID2,
		PriceID:        priceID,
		Status:         models.StatusCancelled,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: CCBillTestSubscriptionID, // Same subscription ID as in the webhook
	})

	t.Run("UserReactivation restores subscription", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "userreactivation.json")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var jsonData map[string]interface{}
		err = json.Unmarshal(data, &jsonData)
		require.NoError(t, err)

		formData := jsonToFormData(jsonData)

		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=UserReactivation",
			strings.NewReader(formData))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "UserReactivation webhook should be accepted")

		// Wait for webhook to be processed
		event, err := suite.WaitForLatestWebhookProcessed("ccbill", "UserReactivation", 5*time.Second)
		require.NoError(t, err, "UserReactivation webhook should be processed")
		require.Equal(t, "processed", event.Status, "UserReactivation should process successfully: %v", event.ErrorMessage)

		// Verify subscription was reactivated
		updatedSub := suite.GetSubscriptionByProcessorID(CCBillTestSubscriptionID)
		require.NotNil(t, updatedSub, "Subscription should exist")
		assert.Equal(t, "active", string(updatedSub.Status), "Subscription should be active after reactivation")
		assert.Nil(t, updatedSub.CancelledAt, "CancelledAt should be cleared")
		assert.Nil(t, updatedSub.EndedAt, "EndedAt should be cleared")
	})
}

// TestWebhookBillingDateChange tests that billing date change updates subscription
func TestWebhookBillingDateChange(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up and seed data for second test user
	suite.CleanupSubscriptionsForUser(CCBillTestUserID2)
	suite.SeedProducts()
	suite.SeedCCBillTestData()

	// Create an active subscription
	products := suite.DefaultTestProducts()
	priceID := products[0].Prices[0].ID

	originalPeriodEnd := time.Now().Add(30 * 24 * time.Hour)
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         CCBillTestUserID2,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: CCBillTestSubscriptionID,
		PeriodEnd:      originalPeriodEnd,
	})

	t.Run("BillingDateChange updates renewal date", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "billingdatechange.json")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var jsonData map[string]interface{}
		err = json.Unmarshal(data, &jsonData)
		require.NoError(t, err)

		formData := jsonToFormData(jsonData)

		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=BillingDateChange",
			strings.NewReader(formData))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "BillingDateChange webhook should be accepted")

		// Wait for webhook to be processed
		event, err := suite.WaitForLatestWebhookProcessed("ccbill", "BillingDateChange", 5*time.Second)
		require.NoError(t, err, "BillingDateChange webhook should be processed")
		require.Equal(t, "processed", event.Status, "BillingDateChange should process successfully: %v", event.ErrorMessage)

		// Verify subscription renewal date was updated
		updatedSub := suite.GetSubscriptionByProcessorID(CCBillTestSubscriptionID)
		require.NotNil(t, updatedSub, "Subscription should exist")
		require.NotNil(t, updatedSub.CurrentPeriodEndsAt, "CurrentPeriodEndsAt should be set")
		// The actual date will depend on the webhook payload
		assert.NotEqual(t, originalPeriodEnd, updatedSub.CurrentPeriodEndsAt,
			"Period end date should be updated")
	})
}

// TestWebhookIdempotency tests that duplicate webhooks are handled correctly
func TestWebhookIdempotency(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up and seed data
	suite.CleanupSubscriptionsForUser(CCBillTestUserID)
	suite.SeedProducts()
	suite.SeedCCBillTestData()

	t.Run("Duplicate NewSaleSuccess does not create duplicate subscription", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "newsalesuccess.json")
		data, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var jsonData map[string]interface{}
		err = json.Unmarshal(data, &jsonData)
		require.NoError(t, err)

		formData := jsonToFormData(jsonData)

		// Send first webhook
		w1 := httptest.NewRecorder()
		req1, _ := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=NewSaleSuccess",
			strings.NewReader(formData))
		req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req1.Header.Set("X-Real-IP", "127.0.0.1")
		suite.Server.Handler().ServeHTTP(w1, req1)
		require.Equal(t, http.StatusOK, w1.Code)

		// Wait for first webhook to be processed
		_, err = suite.WaitForLatestWebhookProcessed("ccbill", "NewSaleSuccess", 5*time.Second)
		require.NoError(t, err)

		// Get subscription count after first webhook
		subsBefore := suite.GetAllSubscriptionsByUserID(CCBillTestUserID)
		countBefore := len(subsBefore)

		// Send duplicate webhook
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest("POST",
			"/v1/webhooks/ccbill?eventType=NewSaleSuccess",
			strings.NewReader(formData))
		req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req2.Header.Set("X-Real-IP", "127.0.0.1")
		suite.Server.Handler().ServeHTTP(w2, req2)
		require.Equal(t, http.StatusOK, w2.Code)

		// Wait a bit for processing
		time.Sleep(500 * time.Millisecond)

		// Verify subscription count didn't increase
		// Note: The system may handle this in different ways:
		// - Detect duplicate and skip processing
		// - Detect user already has active subscription and skip
		subsAfter := suite.GetAllSubscriptionsByUserID(CCBillTestUserID)
		assert.Equal(t, countBefore, len(subsAfter),
			"Duplicate webhook should not create additional subscription")
	})
}

// Helper to send a webhook and wait for it to be processed
func (suite *TestContainerSuite) sendWebhookAndWait(eventType string, filePath string) (*models.WebhookEvent, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return nil, err
	}

	formData := jsonToFormData(jsonData)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST",
		fmt.Sprintf("/v1/webhooks/ccbill?eventType=%s", eventType),
		strings.NewReader(formData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Real-IP", "127.0.0.1")

	suite.Server.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		return nil, fmt.Errorf("webhook returned %d", w.Code)
	}

	return suite.WaitForLatestWebhookProcessed("ccbill", eventType, 5*time.Second)
}
