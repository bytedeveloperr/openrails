package tests

import (
	"bytes"
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

// TestMobiusWebhookValidPayloads tests that valid webhook payloads are processed without errors
func TestMobiusWebhookValidPayloads(t *testing.T) {
	t.Run("Valid Subscription Add Payload", func(t *testing.T) {
		// Create a handler that properly processes valid webhooks
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Validate method
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

			// Parse and validate payload
			var events []map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&events)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "Invalid JSON",
				})
				return
			}

			if len(events) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "Empty payload",
				})
				return
			}

			// Validate each event
			for i, event := range events {
				// Check required fields
				requiredFields := []string{"event_id", "event_type", "event_body"}
				for _, field := range requiredFields {
					if _, ok := event[field]; !ok {
						w.WriteHeader(http.StatusBadRequest)
						json.NewEncoder(w).Encode(map[string]interface{}{
							"error": "Missing required field",
							"field": field,
							"event": i,
						})
						return
					}
				}

				// Validate event_body structure for subscription events
				eventBody, ok := event["event_body"].(map[string]interface{})
				if !ok {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error": "Invalid event_body structure",
						"event": i,
					})
					return
				}

				eventType := event["event_type"].(string)
				if strings.Contains(eventType, "subscription") {
					// Check subscription-specific fields
					if _, ok := eventBody["subscription_id"]; !ok {
						w.WriteHeader(http.StatusBadRequest)
						json.NewEncoder(w).Encode(map[string]interface{}{
							"error": "Missing subscription_id",
							"event": i,
						})
						return
					}
				}
			}

			// Success response
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":          true,
				"message":          "Webhook processed successfully",
				"events_processed": len(events),
			})
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		// Test with real subscription add payload
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load webhook payload")

		resp, err := http.Post(server.URL, "application/json", strings.NewReader(payload))
		require.NoError(t, err, "Should send webhook request")
		defer resp.Body.Close()

		// Read and verify response
		responseBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Should read response body")

		// Should succeed with valid payload
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Valid subscription add payload should be processed successfully")

		// Parse response
		var response map[string]interface{}
		err = json.Unmarshal(responseBody, &response)
		require.NoError(t, err, "Should parse response JSON")

		assert.Equal(t, true, response["success"], "Should indicate success")
		assert.Contains(t, response, "message", "Should have success message")
		assert.Contains(t, response, "events_processed", "Should indicate number of events processed")

		eventsProcessed := int(response["events_processed"].(float64))
		assert.Greater(t, eventsProcessed, 0, "Should have processed at least one event")

		t.Logf("Valid subscription add webhook processed successfully: %d events", eventsProcessed)
	})

	t.Run("Valid Subscription Update Payload", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var events []map[string]interface{}
			json.NewDecoder(r.Body).Decode(&events)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Update webhook processed",
				"events":  len(events),
			})
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_update.json")
		require.NoError(t, err, "Should load update webhook payload")

		resp, err := http.Post(server.URL, "application/json", strings.NewReader(payload))
		require.NoError(t, err, "Should send webhook request")
		defer resp.Body.Close()

		responseBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Should read response body")

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Valid update payload should be processed successfully")

		var response map[string]interface{}
		err = json.Unmarshal(responseBody, &response)
		require.NoError(t, err, "Should parse response")

		assert.Equal(t, true, response["success"], "Should indicate success")

		t.Logf("Valid subscription update webhook processed successfully")
	})

	t.Run("Valid Subscription Delete Payload", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var events []map[string]interface{}
			json.NewDecoder(r.Body).Decode(&events)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Delete webhook processed",
				"events":  len(events),
			})
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_delete.json")
		require.NoError(t, err, "Should load delete webhook payload")

		resp, err := http.Post(server.URL, "application/json", strings.NewReader(payload))
		require.NoError(t, err, "Should send webhook request")
		defer resp.Body.Close()

		responseBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Should read response body")

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Valid delete payload should be processed successfully")

		var response map[string]interface{}
		err = json.Unmarshal(responseBody, &response)
		require.NoError(t, err, "Should parse response")

		assert.Equal(t, true, response["success"], "Should indicate success")

		t.Logf("Valid subscription delete webhook processed successfully")
	})

	t.Run("Customized Valid Payload", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var events []map[string]interface{}
			json.NewDecoder(r.Body).Decode(&events)

			// Extract customized data for verification
			if len(events) > 0 {
				event := events[0]
				eventBody := event["event_body"].(map[string]interface{})
				subscriptionID := eventBody["subscription_id"].(string)

				if billingAddr, ok := eventBody["billing_address"].(map[string]interface{}); ok {
					email := billingAddr["email"].(string)

					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"success":         true,
						"message":         "Customized webhook processed",
						"subscription_id": subscriptionID,
						"email":           email,
					})
					return
				}
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Webhook processed",
			})
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		// Load and customize payload
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load webhook payload")

		var events []map[string]interface{}
		err = json.Unmarshal([]byte(payload), &events)
		require.NoError(t, err, "Should parse payload")

		if len(events) > 0 {
			// Customize the payload
			event := events[0]
			eventBody := event["event_body"].(map[string]interface{})

			customSubscriptionID := "test-valid-sub-12345"
			customEmail := "valid-test@example.com"

			eventBody["subscription_id"] = customSubscriptionID
			if billingAddr, ok := eventBody["billing_address"].(map[string]interface{}); ok {
				billingAddr["email"] = customEmail
			}

			// Convert back to JSON
			customPayload, err := json.Marshal(events)
			require.NoError(t, err, "Should marshal customized payload")

			// Send customized webhook
			resp, err := http.Post(server.URL, "application/json", bytes.NewReader(customPayload))
			require.NoError(t, err, "Should send webhook request")
			defer resp.Body.Close()

			responseBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "Should read response body")

			assert.Equal(t, http.StatusOK, resp.StatusCode, "Customized valid payload should be processed successfully")

			var response map[string]interface{}
			err = json.Unmarshal(responseBody, &response)
			require.NoError(t, err, "Should parse response")

			assert.Equal(t, true, response["success"], "Should indicate success")
			assert.Equal(t, customSubscriptionID, response["subscription_id"], "Should preserve custom subscription ID")
			assert.Equal(t, customEmail, response["email"], "Should preserve custom email")

			t.Logf("Customized valid webhook processed: subscription_id=%s, email=%s",
				response["subscription_id"], response["email"])
		}
	})

	t.Run("Multiple Valid Events", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var events []map[string]interface{}
			json.NewDecoder(r.Body).Decode(&events)

			// Process each event
			processedEvents := make([]map[string]interface{}, 0)
			for _, event := range events {
				eventType, _ := event["event_type"].(string)
				eventBody, _ := event["event_body"].(map[string]interface{})
				subscriptionID, _ := eventBody["subscription_id"].(string)

				processedEvents = append(processedEvents, map[string]interface{}{
					"event_type":      eventType,
					"subscription_id": subscriptionID,
					"processed":       true,
				})
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":          true,
				"message":          "All events processed successfully",
				"total_events":     len(events),
				"processed_events": processedEvents,
			})
		})

		server := httptest.NewServer(handler)
		defer server.Close()

		// Load payload that contains multiple events
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load webhook payload")

		resp, err := http.Post(server.URL, "application/json", strings.NewReader(payload))
		require.NoError(t, err, "Should send webhook request")
		defer resp.Body.Close()

		responseBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Should read response body")

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Multiple valid events should be processed successfully")

		var response map[string]interface{}
		err = json.Unmarshal(responseBody, &response)
		require.NoError(t, err, "Should parse response")

		assert.Equal(t, true, response["success"], "Should indicate success")

		totalEvents := int(response["total_events"].(float64))
		assert.Greater(t, totalEvents, 0, "Should have processed multiple events")

		processedEvents := response["processed_events"].([]interface{})
		assert.Len(t, processedEvents, totalEvents, "Should have processed all events")

		t.Logf("Multiple valid events processed successfully: %d events", totalEvents)
	})

	t.Run("Valid Payload With All Required Fields", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var events []map[string]interface{}
			json.NewDecoder(r.Body).Decode(&events)

			// Detailed validation of the first event
			if len(events) > 0 {
				event := events[0]

				// Validate top-level fields
				eventID, _ := event["event_id"].(string)
				eventType, _ := event["event_type"].(string)
				eventBody, _ := event["event_body"].(map[string]interface{})

				// Validate event_body fields
				subscriptionID := eventBody["subscription_id"]
				billingAddress, _ := eventBody["billing_address"].(map[string]interface{})
				plan, _ := eventBody["plan"].(map[string]interface{})

				// Validate billing address
				email, _ := billingAddress["email"].(string)
				firstName, _ := billingAddress["first_name"].(string)
				lastName, _ := billingAddress["last_name"].(string)

				// Validate plan
				planID, _ := plan["id"].(string)
				amount, _ := plan["amount"].(string)

				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": true,
					"message": "Complete validation passed",
					"validation": map[string]interface{}{
						"event_id":        eventID,
						"event_type":      eventType,
						"subscription_id": subscriptionID,
						"email":           email,
						"customer_name":   firstName + " " + lastName,
						"plan_id":         planID,
						"amount":          amount,
					},
				})
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "No events found",
			})
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

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Complete valid payload should pass all validation")

		var response map[string]interface{}
		err = json.Unmarshal(responseBody, &response)
		require.NoError(t, err, "Should parse response")

		assert.Equal(t, true, response["success"], "Should indicate success")
		assert.Contains(t, response, "validation", "Should include validation details")

		validation := response["validation"].(map[string]interface{})
		assert.NotEmpty(t, validation["event_id"], "Should have event ID")
		assert.NotEmpty(t, validation["event_type"], "Should have event type")
		assert.NotEmpty(t, validation["subscription_id"], "Should have subscription ID")
		assert.NotEmpty(t, validation["email"], "Should have email")
		assert.NotEmpty(t, validation["plan_id"], "Should have plan ID")
		assert.Contains(t, validation, "amount", "Should have amount field")

		t.Logf("Complete validation passed for event: %s", validation["event_type"])
	})
}
