package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/doujins-org/doujins-billing/internal/services/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMobiusWebhookAPI tests the Mobius webhook API logic without containers
func TestMobiusWebhookAPI(t *testing.T) {
	t.Run("Webhook Payload Structure Validation", func(t *testing.T) {
		// Load real webhook payload
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load webhook payload")

		// Parse and validate structure
		var events []map[string]interface{}
		err = json.Unmarshal([]byte(payload), &events)
		require.NoError(t, err, "Should parse webhook payload as JSON array")
		assert.Greater(t, len(events), 0, "Should have at least one event")

		// Validate first event structure
		if len(events) > 0 {
			event := events[0]
			assert.Contains(t, event, "event_id", "Should have event_id")
			assert.Contains(t, event, "event_type", "Should have event_type")
			assert.Contains(t, event, "event_body", "Should have event_body")

			eventType := event["event_type"].(string)
			assert.Equal(t, "recurring.subscription.add", eventType, "Should be subscription add event")

			t.Logf("Validated webhook payload structure for event type: %s", eventType)
		}
	})

	t.Run("Mock Webhook HTTP Request", func(t *testing.T) {
		// Create a simple mock handler that mimics webhook processing
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Validate request method
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			// Validate content type
			contentType := r.Header.Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}

			// Try to parse the body
			var events []map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&events)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Basic validation
			if len(events) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Check first event has required fields
			event := events[0]
			if _, ok := event["event_id"]; !ok {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if _, ok := event["event_type"]; !ok {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if _, ok := event["event_body"]; !ok {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Success response
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Webhook processed successfully",
			})
		})

		// Create test server
		server := httptest.NewServer(handler)
		defer server.Close()

		// Test with valid webhook payload
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load webhook payload")

		resp, err := http.Post(server.URL, "application/json", strings.NewReader(payload))
		require.NoError(t, err, "Should send webhook request")
		defer resp.Body.Close()

		// Verify response
		if resp.StatusCode != http.StatusOK {
			responseBody, _ := io.ReadAll(resp.Body)
			t.Logf("Mock webhook failed with status %d: %s", resp.StatusCode, string(responseBody))
		}
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Should process valid webhook successfully")

		// Parse and verify response
		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err, "Should parse response")
		assert.Equal(t, true, response["success"], "Should indicate success")
		assert.Contains(t, response, "message", "Should have success message")

		t.Logf("Mock webhook processed successfully: %s", response["message"])
	})

	t.Run("Mock Webhook Error Cases", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			contentType := r.Header.Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}

			var events []map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&events)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusOK)
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		// Test wrong method
		req, _ := http.NewRequest("GET", server.URL, nil)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "Should send request")
		resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "Should reject GET method")

		// Test wrong content type
		payload, _ := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		resp, err = http.Post(server.URL, "text/plain", strings.NewReader(payload))
		require.NoError(t, err, "Should send request")
		resp.Body.Close()
		assert.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode, "Should reject wrong content type")

		// Test invalid JSON
		resp, err = http.Post(server.URL, "application/json", strings.NewReader(`{"invalid": json}`))
		require.NoError(t, err, "Should send request")
		resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject invalid JSON")

		t.Log("All error cases handled correctly")
	})

	t.Run("Webhook Payload Customization for Testing", func(t *testing.T) {
		// Load base payload
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load webhook payload")

		// Parse and customize
		var events []map[string]interface{}
		err = json.Unmarshal([]byte(payload), &events)
		require.NoError(t, err, "Should parse payload")

		if len(events) > 0 {
			event := events[0]
			eventBody := event["event_body"].(map[string]interface{})

			// Customize subscription ID
			originalSubID := eventBody["subscription_id"]
			testSubID := "test-api-sub-12345"
			eventBody["subscription_id"] = testSubID

			// Customize email
			if billingAddr, ok := eventBody["billing_address"].(map[string]interface{}); ok {
				billingAddr["email"] = "api-test@example.com"
			}

			// Convert back to JSON
			customizedPayload, err := json.Marshal(events)
			require.NoError(t, err, "Should marshal customized payload")

			// Verify customizations
			assert.Contains(t, string(customizedPayload), testSubID, "Should contain custom subscription ID")
			assert.Contains(t, string(customizedPayload), "api-test@example.com", "Should contain custom email")
			assert.NotContains(t, string(customizedPayload), originalSubID, "Should not contain original subscription ID")

			t.Logf("Successfully customized webhook payload for API testing")
		}
	})

	t.Run("Multiple Event Processing", func(t *testing.T) {
		// Load payload with multiple events
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load webhook payload")

		var events []map[string]interface{}
		err = json.Unmarshal([]byte(payload), &events)
		require.NoError(t, err, "Should parse payload")

		originalEventCount := len(events)
		t.Logf("Original payload has %d events", originalEventCount)

		// Create handler that counts events
		var processedEvents int
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var events []map[string]interface{}
			json.NewDecoder(r.Body).Decode(&events)
			processedEvents = len(events)
			w.WriteHeader(http.StatusOK)
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		// Send webhook
		resp, err := http.Post(server.URL, "application/json", strings.NewReader(payload))
		require.NoError(t, err, "Should send webhook")
		resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Should process webhook")
		assert.Equal(t, originalEventCount, processedEvents, "Should process all events")

		t.Logf("Successfully processed %d events", processedEvents)
	})
}
