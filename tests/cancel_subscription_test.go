//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
)

// TestCancelSubscriptionRequiresAuth tests that cancel endpoint requires authentication
func TestCancelSubscriptionRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		body := map[string]string{"feedback": "test feedback"}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("returns 401 with invalid token", func(t *testing.T) {
		body := map[string]string{"feedback": "test feedback"}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer invalid-token")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestCancelSubscriptionNoActiveSubscription tests cancel with no active subscription
func TestCancelSubscriptionNoActiveSubscription(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Don't create any subscription for the user

	t.Run("returns error when no subscription exists", func(t *testing.T) {
		body := map[string]string{"feedback": "test feedback"}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return error when no subscription")
	})
}

// TestCancelSubscriptionCCBill tests that CCBill subscriptions cannot be cancelled via API
func TestCancelSubscriptionCCBill(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products and create a CCBill subscription
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create an active CCBill subscription for the test user
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: "test-ccbill-sub-" + t.Name(),
	})

	t.Run("returns error for CCBill subscription", func(t *testing.T) {
		body := map[string]string{"feedback": "I want to cancel"}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should fail because CCBill subscriptions cannot be cancelled via API
		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should fail for CCBill subscription")

		// Verify response contains error about processor
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		message, ok := response["message"].(string)
		require.True(t, ok, "Response should have a message field")
		assert.Contains(t, message, "ccbill", "Error should mention CCBill processor")
	})
}

// TestCancelSubscriptionNMI tests cancelling NMI subscriptions
func TestCancelSubscriptionNMI(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products and create an NMI subscription
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create an active NMI subscription for the test user
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorNMI,
		ProcessorSubID: "test-nmi-sub-" + t.Name(),
	})

	t.Run("succeeds for NMI subscription", func(t *testing.T) {
		body := map[string]string{"feedback": "Too expensive"}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should succeed (NMI clients not configured in test, but that's not a hard failure)
		assert.Equal(t, http.StatusOK, w.Code, "Should succeed for NMI subscription")

		// Verify success message
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "cancelled", "Response should confirm cancellation")
	})
}

// TestCancelSubscriptionEmptyFeedback tests cancellation with empty feedback
func TestCancelSubscriptionEmptyFeedback(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products and create an NMI subscription
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create an active NMI subscription
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorNMI,
		ProcessorSubID: "test-nmi-sub-empty-" + t.Name(),
	})

	t.Run("succeeds without feedback", func(t *testing.T) {
		// Empty body
		body := map[string]string{}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Should succeed without feedback")
	})
}

// TestCancelSubscriptionAlreadyCancelled tests cancelling an already cancelled subscription
func TestCancelSubscriptionAlreadyCancelled(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products and create a cancelled subscription
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create a cancelled NMI subscription
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusCancelled,
		Processor:      models.ProcessorNMI,
		ProcessorSubID: "test-nmi-cancelled-" + t.Name(),
	})

	t.Run("fails for already cancelled subscription", func(t *testing.T) {
		body := map[string]string{"feedback": "test"}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should fail because GetActiveSubscription won't find a cancelled subscription
		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should fail for cancelled subscription")
	})
}
