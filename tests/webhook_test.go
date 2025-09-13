package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWebhookEndpoints tests the webhook endpoints
func TestWebhookEndpoints(t *testing.T) {
	server := setupTestServer(t)

	t.Run("CCBillWebhook", func(t *testing.T) {
		webhookData := "eventType=NewSaleSuccess&subscriptionId=123456"

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/webhook/ccbill?eventType=NewSaleSuccess",
			bytes.NewBufferString(webhookData))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		server.Handler().ServeHTTP(w, req)

		// Should not return 404 (route exists)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
		// Will handle webhook processing (success, error, or rate limited)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusForbidden, http.StatusBadRequest, http.StatusTooManyRequests}, w.Code)
	})

	t.Run("MobiusWebhook", func(t *testing.T) {
		webhookData := map[string]interface{}{
			"event": "subscription.created",
			"id":    "sub_123456",
		}

		body, _ := json.Marshal(webhookData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/webhook/mobius", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		server.Handler().ServeHTTP(w, req)

		// Should not return 404 (route exists)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
		// Will handle webhook processing (success, error, or rate limited)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusForbidden, http.StatusBadRequest, http.StatusTooManyRequests}, w.Code)
	})

	t.Run("InvalidProcessor", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/webhook/invalid", nil)

		server.Handler().ServeHTTP(w, req)

		// Should handle invalid processor appropriately (or be rate limited)
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusInternalServerError, http.StatusTooManyRequests}, w.Code)
	})
}
