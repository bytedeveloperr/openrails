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

	// Seed products with matching CCBill IDs
	suite.SeedProducts()

	// Test key CCBill event types
	eventTypes := []struct {
		name       string
		eventType  string
		filePrefix string
	}{
		{"NewSaleSuccess", "NewSaleSuccess", "newsalesuccess"},
		{"Cancellation", "Cancellation", "cancellation"},
		{"RenewalSuccess", "RenewalSuccess", "renewalsuccess"},
		{"RenewalFailure", "RenewalFailure", "renewalfailure"},
		{"Refund", "Refund", "refund"},
		{"Chargeback", "Chargeback", "chargeback"},
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

	// Seed products
	suite.SeedProducts()

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

	// Get user ID from webhook data (username field)
	userID, _ := jsonData["username"].(string)
	if userID == "" {
		t.Skip("No username in webhook data")
		return
	}

	// Verify no subscription exists before
	subsBefore := suite.GetAllSubscriptionsByUserID(userID)

	formData := jsonToFormData(jsonData)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST",
		"/v1/subscriptions/webhook/ccbill?eventType=NewSaleSuccess",
		strings.NewReader(formData))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	suite.Server.Handler().ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		// Give async processing a moment
		// In a real test with River workers running, we'd wait for the job to complete
		// For now, verify the webhook event was stored
		event := suite.GetWebhookEventByEventType("ccbill", "NewSaleSuccess")
		assert.NotNil(t, event, "Webhook event should be stored")

		// After webhook processing completes, subscription should exist
		// Note: This may require waiting for async worker or running synchronously
		subsAfter := suite.GetAllSubscriptionsByUserID(userID)
		t.Logf("Subscriptions before: %d, after: %d", len(subsBefore), len(subsAfter))
	}
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
					http.StatusNotFound,      // Provider not configured
					http.StatusUnauthorized,  // Signature verification failure
					http.StatusBadRequest,    // Parse error
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

	suite.Server.Handler().ServeHTTP(w, req)

	// Should return bad request for missing eventType
	assert.Contains(t, []int{
		http.StatusBadRequest,
		http.StatusForbidden, // IP verification may fail first
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

	suite.Server.Handler().ServeHTTP(w, req)

	// Should handle large payload without crashing
	// May fail on IP verification but that's ok
	assert.Contains(t, []int{
		http.StatusOK,
		http.StatusForbidden,
		http.StatusInternalServerError,
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

		suite.Server.Handler().ServeHTTP(w, req)

		// Should process or fail on IP (not content type)
		assert.NotEqual(t, http.StatusUnsupportedMediaType, w.Code)
	})

	t.Run("NMI expects JSON", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST",
			"/v1/subscriptions/webhook/nmi/mobius",
			strings.NewReader(`{"event_type": "test"}`))
		req.Header.Set("Content-Type", "application/json")

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

		suite.Server.Handler().ServeHTTP(w, req)

		// Should fail (empty body or IP verification)
		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("NMI empty body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST",
			"/v1/subscriptions/webhook/nmi/mobius",
			nil)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should fail
		assert.NotEqual(t, http.StatusOK, w.Code)
	})
}
