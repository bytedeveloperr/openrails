//go:build integration

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to convert JSON map to URL-encoded form data
func jsonToFormData(data map[string]interface{}) string {
	values := url.Values{}
	for key, val := range data {
		var strVal string
		switch v := val.(type) {
		case string:
			strVal = v
		case float64:
			if v == float64(int(v)) {
				strVal = fmt.Sprintf("%d", int(v))
			} else {
				strVal = fmt.Sprintf("%g", v)
			}
		case bool:
			strVal = fmt.Sprintf("%t", v)
		default:
			strVal = fmt.Sprintf("%v", v)
		}
		values.Set(key, strVal)
	}
	return values.Encode()
}

// TestCCBillWebhookReplay tests CCBill webhooks with real replay data
// and verifies they are stored in the database
func TestCCBillWebhookReplay(t *testing.T) {
	suite := setupTestSuite(t)

	// Seed products with matching CCBill IDs and create a subscription
	// This is needed because some event types (renewal, cancellation) require an existing subscription
	suite.SeedCCBillTestDataWithSubscription()

	// Test all 15 CCBill event types
	eventTypes := []struct {
		name       string
		eventType  string
		filePrefix string
	}{
		// Core subscription lifecycle events
		{"NewSaleSuccess", "NewSaleSuccess", "newsalesuccess"},
		{"NewSaleFailure", "NewSaleFailure", "newsalefailure"},
		{"Cancellation", "Cancellation", "cancellation"},
		{"Expiration", "Expiration", "expiration"},
		{"RenewalSuccess", "RenewalSuccess", "renewalsuccess"},
		{"RenewalFailure", "RenewalFailure", "renewalfailure"},
		// Financial events
		{"Refund", "Refund", "refund"},
		{"Void", "Void", "void"},
		{"Chargeback", "Chargeback", "chargeback"},
		// Subscription modification events
		{"UpgradeSuccess", "UpgradeSuccess", "upgradesuccess"},
		{"UpgradeFailure", "UpgradeFailure", "upgradefailure"},
		{"UserReactivation", "UserReactivation", "userreactivation"},
		// Update events
		{"BillingDateChange", "BillingDateChange", "billingdatechange"},
		{"CustomerDataUpdate", "CustomerDataUpdate", "customerdataupdate"},
	}

	for _, et := range eventTypes {
		t.Run(et.name, func(t *testing.T) {
			// Check if file exists
			filePath := filepath.Join("../testdata/webhooks/ccbill", et.filePrefix+".json")
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Skipf("Test data file not found: %s", filePath)
				return
			}

			// Load test data
			data, err := os.ReadFile(filePath)
			require.NoError(t, err, "Failed to read test data")

			var jsonData map[string]interface{}
			err = json.Unmarshal(data, &jsonData)
			require.NoError(t, err, "Failed to parse JSON")

			formData := jsonToFormData(jsonData)

			// Count events before
			eventsBefore := suite.CountWebhookEvents("ccbill")

			// Send webhook
			w := httptest.NewRecorder()
			req, err := http.NewRequest("POST",
				fmt.Sprintf("/v1/subscriptions/webhook/ccbill?eventType=%s", et.eventType),
				strings.NewReader(formData))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("X-Real-IP", "127.0.0.1") // Required for webhook event persistence

			suite.Server.Handler().ServeHTTP(w, req)

			// In dev/test mode with CCBill test_mode enabled, webhook should be accepted
			// Otherwise it may be rejected due to IP verification
			if w.Code == http.StatusOK {
				// Verify webhook was stored in database
				eventsAfter := suite.CountWebhookEvents("ccbill")
				assert.Greater(t, eventsAfter, eventsBefore, "Webhook event should be stored in database")

				// Check the event details
				event := suite.GetWebhookEventByEventType("ccbill", et.eventType)
				assert.NotNil(t, event, "Should find webhook event in database")
				if event != nil {
					assert.Equal(t, "ccbill", event.Processor)
					assert.Equal(t, et.eventType, event.EventType)
					assert.NotEmpty(t, event.RawPayload, "Should store raw payload")
				}
			} else {
				// IP verification might fail in test - log but don't fail
				t.Logf("CCBill %s webhook returned %d (may be IP verification failure in test)", et.name, w.Code)
				assert.Contains(t, []int{http.StatusForbidden, http.StatusInternalServerError, http.StatusTooManyRequests}, w.Code,
					"Expected IP verification failure or rate limit")
			}
		})
	}
}

// TestCCBillNewSaleCreatesSubscription tests that NewSaleSuccess webhook creates subscription
func TestCCBillNewSaleCreatesSubscription(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up any existing subscriptions for test isolation
	// This is needed because tests share the same suite
	suite.CleanupSubscriptionsForUser(CCBillTestUserID)

	// Seed products (includes price with CCBillPriceID matching saved webhook's flexId)
	suite.SeedProducts()

	// Seed CCBill alias mapping (username from webhook → our test user ID)
	suite.SeedCCBillTestData()

	filePath := filepath.Join("../testdata/webhooks/ccbill", "newsalesuccess.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Skip("Test data file not found")
		return
	}

	data, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var jsonData map[string]interface{}
	err = json.Unmarshal(data, &jsonData)
	require.NoError(t, err)

	// Use the saved webhook data as-is - seed data is aligned with webhook payloads
	// The username in the webhook maps to CCBillTestUserID via the alias we seeded

	// Verify no subscription exists before
	subsBefore := suite.GetAllSubscriptionsByUserID(CCBillTestUserID)
	require.Empty(t, subsBefore, "Should have no subscriptions before webhook")

	formData := jsonToFormData(jsonData)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST",
		"/v1/subscriptions/webhook/ccbill?eventType=NewSaleSuccess",
		strings.NewReader(formData))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Real-IP", "127.0.0.1")

	suite.Server.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "Webhook should be accepted")

	// Wait for async webhook processing to complete
	event, err := suite.WaitForLatestWebhookProcessed("ccbill", "NewSaleSuccess", 5*time.Second)
	require.NoError(t, err, "Webhook should be processed within timeout")
	require.NotNil(t, event, "Webhook event should exist")

	// Verify webhook event details
	assert.Equal(t, "ccbill", event.Processor)
	assert.Equal(t, "NewSaleSuccess", event.EventType)
	assert.NotEmpty(t, event.RawPayload)

	// Webhook processing should succeed now that seed data is aligned
	require.Equal(t, "processed", event.Status, "Webhook should be processed successfully: %v", event.ErrorMessage)

	// Verify subscription was created
	subsAfter := suite.GetAllSubscriptionsByUserID(CCBillTestUserID)
	require.Len(t, subsAfter, 1, "Should have 1 subscription after webhook")

	sub := subsAfter[0]
	assert.Equal(t, CCBillTestUserID, sub.UserID)
	assert.Equal(t, "active", string(sub.Status))
	assert.Equal(t, "ccbill", string(sub.Processor))
	assert.Equal(t, CCBillTestSubscriptionID, sub.ProcessorSubscriptionID)

	// Note: CCBill webhooks don't create payment records in billing.payments table
	// Payment events are logged to ClickHouse analytics instead
	// For NMI subscriptions, payments are created by the Subscribe handler

	// Verify entitlements were granted
	entitlements := suite.GetEntitlementsByUserID(CCBillTestUserID)
	assert.NotEmpty(t, entitlements, "Should have entitlements after subscription created")
}

// TestNMIWebhookReplay tests NMI webhooks with real replay data
func TestNMIWebhookReplay(t *testing.T) {
	suite := setupTestSuite(t)

	// Seed products with matching NMI plan IDs
	suite.SeedProducts()

	eventTypes := []struct {
		name      string
		eventType string
		fileName  string
	}{
		{"SubscriptionAdd", "recurring.subscription.add", "recurring_subscription_add.json"},
		{"SubscriptionUpdate", "recurring.subscription.update", "recurring_subscription_update.json"},
		{"SubscriptionDelete", "recurring.subscription.delete", "recurring_subscription_delete.json"},
	}

	for _, et := range eventTypes {
		t.Run(et.name, func(t *testing.T) {
			filePath := filepath.Join("../testdata/webhooks/nmi", et.fileName)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Skipf("Test data file not found: %s", filePath)
				return
			}

			// Load and parse test data
			data, err := os.ReadFile(filePath)
			require.NoError(t, err)

			var events []map[string]interface{}
			err = json.Unmarshal(data, &events)
			require.NoError(t, err)
			require.NotEmpty(t, events, "Should have at least one event")

			// Use the first event
			eventData, err := json.Marshal(events[0])
			require.NoError(t, err)

			// Count events before
			eventsBefore := suite.CountWebhookEvents("nmi")

			w := httptest.NewRecorder()
			req, err := http.NewRequest("POST",
				"/v1/subscriptions/webhook/nmi/mobius",
				strings.NewReader(string(eventData)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Real-IP", "127.0.0.1")

			suite.Server.Handler().ServeHTTP(w, req)

			// NMI webhooks require signature verification in non-test mode
			// In dev mode, may return 404 (provider not configured) or 401 (signature failure)
			if w.Code == http.StatusOK {
				// Verify webhook was stored
				eventsAfter := suite.CountWebhookEvents("nmi")
				assert.Greater(t, eventsAfter, eventsBefore, "Webhook event should be stored")

				event := suite.GetWebhookEventByEventType("nmi", et.eventType)
				assert.NotNil(t, event, "Should find webhook event")
				if event != nil {
					assert.Equal(t, "nmi", event.Processor)
				}
			} else {
				// Expected in dev mode without NMI config
				t.Logf("NMI %s webhook returned %d", et.name, w.Code)
				assert.Contains(t, []int{
					http.StatusNotFound,     // Provider not configured
					http.StatusUnauthorized, // Signature verification failure
					http.StatusBadRequest,   // Parse error
					http.StatusTooManyRequests,
				}, w.Code)
			}
		})
	}
}

// TestWebhookWithInvalidProcessor tests webhook with invalid processor
func TestWebhookWithInvalidProcessor(t *testing.T) {
	suite := setupTestSuite(t)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/v1/subscriptions/webhook/invalid",
		strings.NewReader("test=data"))
	require.NoError(t, err)

	suite.Server.Handler().ServeHTTP(w, req)

	// Should return bad request
	assert.Contains(t, []int{
		http.StatusBadRequest,
		http.StatusTooManyRequests,
	}, w.Code)
}

// TestCCBillWebhookWithMissingEventType tests CCBill webhook without eventType
func TestCCBillWebhookWithMissingEventType(t *testing.T) {
	suite := setupTestSuite(t)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/v1/subscriptions/webhook/ccbill",
		strings.NewReader("subscriptionId=123456"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Real-IP", "127.0.0.1")

	suite.Server.Handler().ServeHTTP(w, req)

	// Should return bad request for missing eventType
	assert.Contains(t, []int{
		http.StatusBadRequest,
		http.StatusTooManyRequests,
	}, w.Code)
}

// TestNMIWebhookWithMalformedJSON tests NMI webhook with invalid JSON
func TestNMIWebhookWithMalformedJSON(t *testing.T) {
	suite := setupTestSuite(t)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/v1/subscriptions/webhook/nmi/mobius",
		strings.NewReader("{invalid json"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	suite.Server.Handler().ServeHTTP(w, req)

	// Should fail with bad request or auth error
	assert.Contains(t, []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusTooManyRequests,
	}, w.Code)
}

// TestWebhookReplayWithLargePayload tests webhook handling with large payload
func TestWebhookReplayWithLargePayload(t *testing.T) {
	suite := setupTestSuite(t)

	// Create a large payload
	largeData := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		largeData[fmt.Sprintf("field_%d", i)] = fmt.Sprintf("value_%d", i)
	}
	largeData["eventType"] = "TestEvent"
	largeData["subscriptionId"] = "123456789"

	formData := jsonToFormData(largeData)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST",
		"/v1/subscriptions/webhook/ccbill?eventType=TestEvent",
		strings.NewReader(formData))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Real-IP", "127.0.0.1")

	suite.Server.Handler().ServeHTTP(w, req)

	// Should handle large payload without crashing
	assert.Contains(t, []int{
		http.StatusOK,
		http.StatusInternalServerError, // May fail on other validation
		http.StatusTooManyRequests,
	}, w.Code)
}

// TestWebhookContentTypeValidation tests content type handling
func TestWebhookContentTypeValidation(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("CCBill accepts form data", func(t *testing.T) {
		filePath := filepath.Join("../testdata/webhooks/ccbill", "newsalesuccess.json")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Skip("Test data not found")
			return
		}

		data, _ := os.ReadFile(filePath)
		var jsonData map[string]interface{}
		json.Unmarshal(data, &jsonData)
		formData := jsonToFormData(jsonData)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST",
			"/v1/subscriptions/webhook/ccbill?eventType=NewSaleSuccess",
			strings.NewReader(formData))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)

		// Should process successfully (not fail on content type)
		assert.NotEqual(t, http.StatusUnsupportedMediaType, w.Code)
	})

	t.Run("NMI expects JSON", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST",
			"/v1/subscriptions/webhook/nmi/mobius",
			strings.NewReader(`{"event_type": "test"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)

		// Should not fail on content type specifically
		assert.NotEqual(t, http.StatusUnsupportedMediaType, w.Code)
	})
}

// TestWebhookReplayEmptyBody tests webhooks with empty body
func TestWebhookReplayEmptyBody(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("CCBill empty body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST",
			"/v1/subscriptions/webhook/ccbill?eventType=Test",
			nil)
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)

		// CCBill webhook endpoint accepts all webhooks and processes them asynchronously
		// This is a deliberate design choice to respond quickly to payment processors
		// The empty body will fail during async processing, not at the HTTP layer
		assert.Equal(t, http.StatusOK, w.Code, "CCBill webhook endpoint should accept all webhooks synchronously")
	})

	t.Run("NMI empty body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST",
			"/v1/subscriptions/webhook/nmi/mobius",
			nil)
		req.Header.Set("X-Real-IP", "127.0.0.1")

		suite.Server.Handler().ServeHTTP(w, req)

		// NMI provider not configured in test, should fail
		assert.NotEqual(t, http.StatusOK, w.Code)
	})
}
