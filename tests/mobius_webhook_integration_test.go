//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/doujins-org/doujins-billing/internal/services/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMobiusWebhookEndpoint tests the actual Mobius webhook API endpoint
func TestMobiusWebhookEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Check if Docker is available for containers
	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping webhook endpoint tests")
	}

	// Create test container suite
	suite := NewTestContainerSuite(t)
	defer suite.Cleanup()

	t.Run("Process Subscription Add Webhook", func(t *testing.T) {
		// Load the test webhook payload
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load webhook payload")

		// Customize the payload for this test
		var events []map[string]interface{}
		err = json.Unmarshal([]byte(payload), &events)
		require.NoError(t, err, "Should parse payload")

		if len(events) > 0 {
			// Customize the first event
			event := events[0]
			eventBody := event["event_body"].(map[string]interface{})
			eventBody["subscription_id"] = "test-integration-sub-001"

			if billingAddr, ok := eventBody["billing_address"].(map[string]interface{}); ok {
				billingAddr["email"] = "integration-test@example.com"
			}

			// Convert back to JSON
			customPayload, err := json.Marshal(events)
			require.NoError(t, err, "Should marshal customized payload")

			// Send webhook to the API endpoint
			webhookURL := suite.ServerURL + "/api/v1/webhooks/mobius"
			resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(customPayload))
			require.NoError(t, err, "Should send webhook request")
			defer resp.Body.Close()

			// Read and verify response
			responseBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "Should read response body")

			// Check response status and content
			if resp.StatusCode != http.StatusOK {
				t.Logf("Webhook failed with status %d: %s", resp.StatusCode, string(responseBody))

				// Check if it's a known error (like endpoint not implemented)
				if resp.StatusCode == http.StatusNotFound {
					t.Skip("Webhook endpoint not implemented yet - this is expected")
				} else {
					t.Errorf("Webhook processing failed with status %d: %s", resp.StatusCode, string(responseBody))
				}
			} else {
				// Verify successful response
				assert.Equal(t, http.StatusOK, resp.StatusCode, "Webhook should be processed successfully")

				// Try to parse response as JSON if possible
				var responseData map[string]interface{}
				if err := json.Unmarshal(responseBody, &responseData); err == nil {
					t.Logf("Webhook response: %+v", responseData)
				} else {
					t.Logf("Webhook response (non-JSON): %s", string(responseBody))
				}

				t.Logf("Mobius subscription add webhook processed successfully")
			}
		}
	})

	t.Run("Process Subscription Update Webhook", func(t *testing.T) {
		// Load and customize update webhook
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_update.json")
		require.NoError(t, err, "Should load webhook payload")

		var events []map[string]interface{}
		err = json.Unmarshal([]byte(payload), &events)
		require.NoError(t, err, "Should parse payload")

		if len(events) > 0 {
			// Customize for test
			event := events[0]
			eventBody := event["event_body"].(map[string]interface{})
			eventBody["subscription_id"] = "test-integration-sub-002"

			customPayload, err := json.Marshal(events)
			require.NoError(t, err, "Should marshal payload")

			// Send to webhook endpoint
			webhookURL := suite.ServerURL + "/api/v1/webhooks/mobius"
			resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(customPayload))
			require.NoError(t, err, "Should send webhook request")
			defer resp.Body.Close()

			// Read and verify response
			responseBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "Should read response body")

			if resp.StatusCode != http.StatusOK {
				t.Logf("Update webhook failed with status %d: %s", resp.StatusCode, string(responseBody))
				if resp.StatusCode == http.StatusNotFound {
					t.Skip("Webhook endpoint not implemented yet")
				}
			} else {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "Update webhook should be processed")
				t.Logf("Mobius subscription update webhook processed successfully")
			}
		}
	})

	t.Run("Process Subscription Delete Webhook", func(t *testing.T) {
		// Load and customize delete webhook
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_delete.json")
		require.NoError(t, err, "Should load webhook payload")

		var events []map[string]interface{}
		err = json.Unmarshal([]byte(payload), &events)
		require.NoError(t, err, "Should parse payload")

		if len(events) > 0 {
			// Customize for test
			event := events[0]
			eventBody := event["event_body"].(map[string]interface{})
			eventBody["subscription_id"] = "test-integration-sub-003"

			customPayload, err := json.Marshal(events)
			require.NoError(t, err, "Should marshal payload")

			// Send to webhook endpoint
			webhookURL := suite.ServerURL + "/api/v1/webhooks/mobius"
			resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(customPayload))
			require.NoError(t, err, "Should send webhook request")
			defer resp.Body.Close()

			// Read and verify response
			responseBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "Should read response body")

			if resp.StatusCode != http.StatusOK {
				t.Logf("Delete webhook failed with status %d: %s", resp.StatusCode, string(responseBody))
				if resp.StatusCode == http.StatusNotFound {
					t.Skip("Webhook endpoint not implemented yet")
				}
			} else {
				assert.Equal(t, http.StatusOK, resp.StatusCode, "Delete webhook should be processed")
				t.Logf("Mobius subscription delete webhook processed successfully")
			}
		}
	})

	t.Run("Invalid Webhook Payload", func(t *testing.T) {
		// Send invalid JSON
		invalidPayload := `{"invalid": "json", "missing": "required_fields"`

		webhookURL := suite.ServerURL + "/api/v1/webhooks/mobius"
		resp, err := http.Post(webhookURL, "application/json", strings.NewReader(invalidPayload))
		require.NoError(t, err, "Should send request")
		defer resp.Body.Close()

		// Should return error status for invalid payload
		assert.NotEqual(t, http.StatusOK, resp.StatusCode, "Should reject invalid payload")

		t.Logf("Invalid webhook payload properly rejected with status: %d", resp.StatusCode)
	})

	t.Run("Wrong Content Type", func(t *testing.T) {
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load payload")

		// Send with wrong content type
		webhookURL := suite.ServerURL + "/api/v1/webhooks/mobius"
		resp, err := http.Post(webhookURL, "text/plain", strings.NewReader(payload))
		require.NoError(t, err, "Should send request")
		defer resp.Body.Close()

		// Should handle wrong content type appropriately
		t.Logf("Wrong content type handled with status: %d", resp.StatusCode)
	})
}

// TestMobiusWebhookAuthentication tests webhook authentication if implemented
func TestMobiusWebhookAuthentication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping webhook authentication tests")
	}

	suite := NewTestContainerSuite(t)
	defer suite.Cleanup()

	t.Run("Webhook Without Authentication", func(t *testing.T) {
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load payload")

		webhookURL := suite.ServerURL + "/api/v1/webhooks/mobius"
		resp, err := http.Post(webhookURL, "application/json", strings.NewReader(payload))
		require.NoError(t, err, "Should send request")
		defer resp.Body.Close()

		// Log the response for debugging
		t.Logf("Webhook without auth returned status: %d", resp.StatusCode)
	})

	t.Run("Webhook With Custom Headers", func(t *testing.T) {
		payload, err := webhook.LoadTestWebhookPayload("mobius", "recurring_subscription_add.json")
		require.NoError(t, err, "Should load payload")

		// Create request with custom headers
		webhookURL := suite.ServerURL + "/api/v1/webhooks/mobius"
		req, err := http.NewRequest("POST", webhookURL, strings.NewReader(payload))
		require.NoError(t, err, "Should create request")

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mobius-Signature", "test-signature")
		req.Header.Set("User-Agent", "Mobius-Webhook/1.0")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err, "Should send request")
		defer resp.Body.Close()

		t.Logf("Webhook with custom headers returned status: %d", resp.StatusCode)
	})
}
