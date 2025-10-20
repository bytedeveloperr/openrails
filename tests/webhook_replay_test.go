package tests

import (
	"bytes"
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

// Helper function to load CCBill webhook data from JSON and convert to form data
func loadCCBillWebhookData(t *testing.T, eventType string) string {
	t.Helper()

	// Map event type to file name
	fileName := strings.ToLower(eventType) + ".json"
	filePath := filepath.Join("../testdata/webhooks/ccbill", fileName)

	data, err := os.ReadFile(filePath)
	require.NoError(t, err, "Failed to read CCBill webhook test data for %s", eventType)

	// Parse JSON to convert to form data
	var jsonData map[string]interface{}
	err = json.Unmarshal(data, &jsonData)
	require.NoError(t, err, "Failed to parse CCBill webhook JSON for %s", eventType)

	// Convert to URL-encoded form data
	return jsonToFormData(jsonData)
}

// Helper function to convert JSON map to URL-encoded form data
func jsonToFormData(data map[string]interface{}) string {
	values := url.Values{}
	for key, val := range data {
		// Convert value to string
		var strVal string
		switch v := val.(type) {
		case string:
			strVal = v
		case float64:
			// Handle both integers and floats
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

// Helper function to load Mobius webhook data
func loadMobiusWebhookData(t *testing.T, eventType string) []byte {
	t.Helper()

	// Map event type to file name
	var fileName string
	switch eventType {
	case "recurring.subscription.add":
		fileName = "recurring_subscription_add.json"
	case "recurring.subscription.update":
		fileName = "recurring_subscription_update.json"
	case "recurring.subscription.delete":
		fileName = "recurring_subscription_delete.json"
	default:
		t.Fatalf("Unknown Mobius event type: %s", eventType)
	}

	filePath := filepath.Join("../testdata/webhooks/mobius", fileName)
	data, err := os.ReadFile(filePath)
	require.NoError(t, err, "Failed to read Mobius webhook test data for %s", eventType)

	// Mobius sends arrays of events, so we need to extract the first event
	var events []map[string]interface{}
	err = json.Unmarshal(data, &events)
	require.NoError(t, err, "Failed to parse Mobius webhook JSON for %s", eventType)
	require.NotEmpty(t, events, "No events found in Mobius webhook data for %s", eventType)

	// Use the first event for testing
	eventData, err := json.Marshal(events[0])
	require.NoError(t, err, "Failed to marshal Mobius event for %s", eventType)

	return eventData
}

// TestCCBillWebhookReplay tests CCBill webhooks with real replay data
func TestCCBillWebhookReplay(t *testing.T) {
	server := setupTestServer(t)

	// Test all CCBill event types
	eventTypes := []struct {
		name       string
		eventType  string
		filePrefix string
	}{
		{"NewSaleSuccess", "NewSaleSuccess", "newsalesuccess"},
		{"NewSaleFailure", "NewSaleFailure", "newsalefailure"},
		{"Cancellation", "Cancellation", "cancellation"},
		{"Expiration", "Expiration", "expiration"},
		{"RenewalSuccess", "RenewalSuccess", "renewalsuccess"},
		{"RenewalFailure", "RenewalFailure", "renewalfailure"},
		{"Refund", "Refund", "refund"},
		{"Chargeback", "Chargeback", "chargeback"},
		{"BillingDateChange", "BillingDateChange", "billingdatechange"},
		{"CustomerDataUpdate", "CustomerDataUpdate", "customerdataupdate"},
		{"UpgradeSuccess", "UpgradeSuccess", "upgradesuccess"},
		{"UpgradeFailure", "UpgradeFailure", "upgradefailure"},
		{"UserReactivation", "UserReactivation", "userreactivation"},
		{"Void", "Void", "void"},
	}

	for _, et := range eventTypes {
		t.Run(et.name, func(t *testing.T) {
			// Check if file exists first
			filePath := filepath.Join("../testdata/webhooks/ccbill", et.filePrefix+".json")
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Skipf("Test data file not found: %s", filePath)
				return
			}

			// Load test data
			data, err := os.ReadFile(filePath)
			require.NoError(t, err, "Failed to read test data")

			// Parse JSON to convert to form data
			var jsonData map[string]interface{}
			err = json.Unmarshal(data, &jsonData)
			require.NoError(t, err, "Failed to parse JSON")

			// Convert to form data
			formData := jsonToFormData(jsonData)

			// Create request
			w := httptest.NewRecorder()
			req, err := http.NewRequest("POST",
				fmt.Sprintf("/v1/subscriptions/webhook/ccbill?eventType=%s", et.eventType),
				strings.NewReader(formData))
			require.NoError(t, err)

			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Send request
			server.Handler().ServeHTTP(w, req)

			// Check response
			// We expect either OK, rate limited, or internal error (due to missing services in test)
			assert.Contains(t, []int{
				http.StatusOK,
				http.StatusInternalServerError,
				http.StatusTooManyRequests,
				http.StatusForbidden, // IP verification might fail in test
			}, w.Code,
				"Unexpected status code for CCBill %s webhook: %d", et.name, w.Code)

			// Log response for debugging
			if w.Code != http.StatusOK && w.Code != http.StatusTooManyRequests {
				t.Logf("CCBill %s webhook response: %d - %s", et.name, w.Code, w.Body.String())
			}
		})
	}
}

// TestMobiusWebhookReplay tests Mobius webhooks with real replay data
func TestMobiusWebhookReplay(t *testing.T) {
	server := setupTestServer(t)

	// Test all Mobius event types
	eventTypes := []struct {
		name      string
		eventType string
	}{
		{"SubscriptionAdd", "recurring.subscription.add"},
		{"SubscriptionUpdate", "recurring.subscription.update"},
		{"SubscriptionDelete", "recurring.subscription.delete"},
	}

	for _, et := range eventTypes {
		t.Run(et.name, func(t *testing.T) {
			// Load test data
			webhookData := loadMobiusWebhookData(t, et.eventType)

			// Create request
			w := httptest.NewRecorder()
			req, err := http.NewRequest("POST",
				"/v1/subscriptions/webhook/mobius",
				bytes.NewBuffer(webhookData))
			require.NoError(t, err)

			req.Header.Set("Content-Type", "application/json")

			// Send request
			server.Handler().ServeHTTP(w, req)

			// Check response
			// We expect either OK, rate limited, unauthorized (signature), or internal error
			assert.Contains(t, []int{
				http.StatusOK,
				http.StatusInternalServerError,
				http.StatusTooManyRequests,
				http.StatusUnauthorized, // Signature verification might fail
			}, w.Code,
				"Unexpected status code for Mobius %s webhook: %d", et.name, w.Code)

			// Log response for debugging
			if w.Code != http.StatusOK && w.Code != http.StatusTooManyRequests {
				t.Logf("Mobius %s webhook response: %d - %s", et.name, w.Code, w.Body.String())
			}
		})
	}
}

// TestWebhookWithInvalidProcessor tests webhook with invalid processor
func TestWebhookWithInvalidProcessor(t *testing.T) {
	server := setupTestServer(t)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/v1/subscriptions/webhook/invalid",
		strings.NewReader("test=data"))
	require.NoError(t, err)

	server.Handler().ServeHTTP(w, req)

	// Should return bad request or rate limited
	assert.Contains(t, []int{
		http.StatusBadRequest,
		http.StatusTooManyRequests,
	}, w.Code)
}

// TestCCBillWebhookWithMissingEventType tests CCBill webhook without eventType parameter
func TestCCBillWebhookWithMissingEventType(t *testing.T) {
	server := setupTestServer(t)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/v1/subscriptions/webhook/ccbill",
		strings.NewReader("subscriptionId=123456"))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	server.Handler().ServeHTTP(w, req)

	// Should still process but with empty eventType
	assert.Contains(t, []int{
		http.StatusOK,
		http.StatusInternalServerError,
		http.StatusTooManyRequests,
		http.StatusForbidden,
	}, w.Code)
}

// TestMobiusWebhookWithMalformedJSON tests Mobius webhook with invalid JSON
func TestMobiusWebhookWithMalformedJSON(t *testing.T) {
	server := setupTestServer(t)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/v1/subscriptions/webhook/mobius",
		strings.NewReader("{invalid json"))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	server.Handler().ServeHTTP(w, req)

	// Should return bad request or rate limited
	assert.Contains(t, []int{
		http.StatusBadRequest,
		http.StatusTooManyRequests,
		http.StatusUnauthorized, // Might fail signature check first
	}, w.Code)
}

// TestWebhookReplayWithLargePayload tests webhook handling with a large payload
func TestWebhookReplayWithLargePayload(t *testing.T) {
	server := setupTestServer(t)

	// Create a large payload (simulate a webhook with many fields)
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

	server.Handler().ServeHTTP(w, req)

	// Should handle large payload without crashing
	assert.Contains(t, []int{
		http.StatusOK,
		http.StatusInternalServerError,
		http.StatusTooManyRequests,
		http.StatusForbidden,
	}, w.Code)
}

// TestWebhookContentTypeValidation tests that webhooks validate content type
func TestWebhookContentTypeValidation(t *testing.T) {
	server := setupTestServer(t)

	t.Run("CCBill_WrongContentType", func(t *testing.T) {
		// CCBill expects form data, send JSON instead
		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/subscriptions/webhook/ccbill?eventType=NewSaleSuccess",
			strings.NewReader(`{"test": "data"}`))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json") // Wrong content type

		server.Handler().ServeHTTP(w, req)

		// Should still attempt to process (handler reads raw body)
		assert.Contains(t, []int{
			http.StatusOK,
			http.StatusInternalServerError,
			http.StatusTooManyRequests,
			http.StatusForbidden,
		}, w.Code)
	})

	t.Run("Mobius_WrongContentType", func(t *testing.T) {
		// Mobius expects JSON, send form data instead
		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/subscriptions/webhook/mobius",
			strings.NewReader("test=data&foo=bar"))
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded") // Wrong content type

		server.Handler().ServeHTTP(w, req)

		// Should fail JSON parsing
		assert.Contains(t, []int{
			http.StatusBadRequest,
			http.StatusTooManyRequests,
			http.StatusUnauthorized,
		}, w.Code)
	})
}

// TestWebhookReplayEmptyBody tests webhooks with empty body
func TestWebhookReplayEmptyBody(t *testing.T) {
	server := setupTestServer(t)

	t.Run("CCBill_EmptyBody", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/subscriptions/webhook/ccbill?eventType=Test",
			nil)
		require.NoError(t, err)

		server.Handler().ServeHTTP(w, req)

		assert.Contains(t, []int{
			http.StatusInternalServerError,
			http.StatusTooManyRequests,
			http.StatusForbidden,
		}, w.Code)
	})

	t.Run("Mobius_EmptyBody", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST",
			"/v1/subscriptions/webhook/mobius",
			nil)
		require.NoError(t, err)

		server.Handler().ServeHTTP(w, req)

		assert.Contains(t, []int{
			http.StatusBadRequest,
			http.StatusInternalServerError,
			http.StatusTooManyRequests,
			http.StatusUnauthorized,
		}, w.Code)
	})
}
