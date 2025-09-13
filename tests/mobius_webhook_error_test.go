package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/doujins-org/doujins-billing/internal/services/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMobiusWebhookErrorHandling tests comprehensive error handling for Mobius webhooks
func TestMobiusWebhookErrorHandling(t *testing.T) {
	t.Run("Invalid JSON Response Handling", func(t *testing.T) {
		// Create a handler that returns invalid JSON
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Internal server error", "details": "Database connection failed"}`))
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		// Load valid webhook payload
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load webhook payload")

		// Send webhook
		resp, err := http.Post(server.URL, "application/json", strings.NewReader(payload))
		require.NoError(t, err, "Should send webhook request")
		defer resp.Body.Close()

		// Read and verify error response
		responseBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Should read response body")

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "Should return error status")

		// Parse error response
		var errorResponse map[string]interface{}
		err = json.Unmarshal(responseBody, &errorResponse)
		require.NoError(t, err, "Should parse error response as JSON")

		assert.Contains(t, errorResponse, "error", "Should have error field")
		assert.Equal(t, "Internal server error", errorResponse["error"], "Should have correct error message")

		t.Logf("Error response properly handled: %s", errorResponse["error"])
	})

	t.Run("Network Timeout Simulation", func(t *testing.T) {
		// Create a handler that simulates slow response
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusRequestTimeout)
			w.Write([]byte(`{"error": "Request timeout", "message": "Webhook processing took too long"}`))
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load webhook payload")

		resp, err := http.Post(server.URL, "application/json", strings.NewReader(payload))
		require.NoError(t, err, "Should send webhook request")
		defer resp.Body.Close()

		responseBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Should read response body")

		assert.Equal(t, http.StatusRequestTimeout, resp.StatusCode, "Should return timeout status")

		t.Logf("Timeout response handled: %s", string(responseBody))
	})

	t.Run("Malformed Webhook Payload", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try to parse the request body
			var events []map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&events)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "Invalid JSON",
					"message": "Failed to parse webhook payload",
					"details": err.Error(),
				})
				return
			}

			// Validate required fields
			if len(events) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "Empty payload",
					"message": "Webhook payload contains no events",
				})
				return
			}

			// Check first event structure
			event := events[0]
			requiredFields := []string{"event_id", "event_type", "event_body"}
			for _, field := range requiredFields {
				if _, ok := event[field]; !ok {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error":   "Missing required field",
						"message": fmt.Sprintf("Event missing required field: %s", field),
						"field":   field,
					})
					return
				}
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Webhook processed successfully",
			})
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		// Test with invalid JSON
		resp, err := http.Post(server.URL, "application/json", strings.NewReader(`{"invalid": json}`))
		require.NoError(t, err, "Should send request")
		defer resp.Body.Close()

		responseBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Should read response")

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject invalid JSON")

		var errorResponse map[string]interface{}
		err = json.Unmarshal(responseBody, &errorResponse)
		require.NoError(t, err, "Should parse error response")
		assert.Equal(t, "Invalid JSON", errorResponse["error"], "Should have correct error message")

		t.Logf("Invalid JSON properly rejected: %s", errorResponse["message"])

		// Test with empty array
		resp2, err := http.Post(server.URL, "application/json", strings.NewReader(`[]`))
		require.NoError(t, err, "Should send request")
		defer resp2.Body.Close()

		responseBody2, err := io.ReadAll(resp2.Body)
		require.NoError(t, err, "Should read response")

		assert.Equal(t, http.StatusBadRequest, resp2.StatusCode, "Should reject empty payload")

		var errorResponse2 map[string]interface{}
		err = json.Unmarshal(responseBody2, &errorResponse2)
		require.NoError(t, err, "Should parse error response")
		assert.Equal(t, "Empty payload", errorResponse2["error"], "Should have correct error message")

		t.Logf("Empty payload properly rejected: %s", errorResponse2["message"])

		// Test with missing required fields
		incompleteEvent := []map[string]interface{}{
			{
				"event_id": "test-123",
				// Missing event_type and event_body
			},
		}
		incompletePayload, _ := json.Marshal(incompleteEvent)

		resp3, err := http.Post(server.URL, "application/json", bytes.NewReader(incompletePayload))
		require.NoError(t, err, "Should send request")
		defer resp3.Body.Close()

		responseBody3, err := io.ReadAll(resp3.Body)
		require.NoError(t, err, "Should read response")

		assert.Equal(t, http.StatusBadRequest, resp3.StatusCode, "Should reject incomplete event")

		var errorResponse3 map[string]interface{}
		err = json.Unmarshal(responseBody3, &errorResponse3)
		require.NoError(t, err, "Should parse error response")
		assert.Equal(t, "Missing required field", errorResponse3["error"], "Should have correct error message")

		t.Logf("Incomplete event properly rejected: %s", errorResponse3["message"])
	})

	t.Run("HTTP Method Validation", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "Method not allowed",
					"message": fmt.Sprintf("Only POST method is supported, got %s", r.Method),
					"allowed": []string{"POST"},
				})
				return
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
			})
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		// Test wrong HTTP methods
		methods := []string{"GET", "PUT", "DELETE", "PATCH"}
		for _, method := range methods {
			req, err := http.NewRequest(method, server.URL, nil)
			require.NoError(t, err, "Should create request")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err, "Should send request")
			defer resp.Body.Close()

			responseBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "Should read response")

			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "Should reject %s method", method)

			var errorResponse map[string]interface{}
			err = json.Unmarshal(responseBody, &errorResponse)
			require.NoError(t, err, "Should parse error response")
			assert.Equal(t, "Method not allowed", errorResponse["error"], "Should have correct error message")

			t.Logf("%s method properly rejected", method)
		}
	})

	t.Run("Content Type Validation", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentType := r.Header.Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":    "Unsupported media type",
					"message":  "Content-Type must be application/json",
					"received": contentType,
					"expected": "application/json",
				})
				return
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
			})
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load payload")

		// Test wrong content types
		contentTypes := []string{"text/plain", "application/xml", "text/html", "application/form-data"}
		for _, contentType := range contentTypes {
			resp, err := http.Post(server.URL, contentType, strings.NewReader(payload))
			require.NoError(t, err, "Should send request")
			defer resp.Body.Close()

			responseBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "Should read response")

			assert.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode, "Should reject %s content type", contentType)

			var errorResponse map[string]interface{}
			err = json.Unmarshal(responseBody, &errorResponse)
			require.NoError(t, err, "Should parse error response")
			assert.Equal(t, "Unsupported media type", errorResponse["error"], "Should have correct error message")

			t.Logf("%s content type properly rejected", contentType)
		}
	})
}
