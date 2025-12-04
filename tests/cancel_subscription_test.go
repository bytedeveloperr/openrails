//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
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

	t.Run("returns 422 with support URL for CCBill subscription", func(t *testing.T) {
		body := map[string]string{"feedback": "I want to cancel"}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should return 422 Unprocessable Entity with CCBill support URL
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, "Should return 422 for CCBill subscription")

		// Verify response contains support URL and error code
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Check for support_url
		supportURL, ok := response["support_url"].(string)
		require.True(t, ok, "Response should have support_url field")
		assert.Equal(t, "https://support.ccbill.com", supportURL, "Should return CCBill support URL")

		// Check for error code
		code, ok := response["code"].(string)
		require.True(t, ok, "Response should have code field")
		assert.Equal(t, "ccbill_cancel_required", code, "Should return ccbill_cancel_required code")

		// Check for error message
		errorMsg, ok := response["error"].(string)
		require.True(t, ok, "Response should have error field")
		assert.Contains(t, errorMsg, "CCBill", "Error should mention CCBill")
	})
}

// TestCancelSubscriptionNMI tests cancelling NMI subscriptions
// TODO: Requires real Mobius test account - see progress.json "nmi-test-account-integration"
// Currently skipped because the test creates a fake subscription ID that doesn't exist in NMI's system,
// so the cancel API call fails with "Transaction not found". Once we have a real Mobius test account,
// we can create real subscriptions and test the full cancel flow.
func TestCancelSubscriptionNMI(t *testing.T) {
	t.Skip("TODO: Requires real Mobius/NMI test account to create subscriptions that can be cancelled")

	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products and create an NMI subscription
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create an active NMI subscription for the test user
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorMobius,
		ProcessorSubID: "test-mobius-sub-" + t.Name(),
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
// TODO: Requires real Mobius test account - see progress.json "nmi-test-account-integration"
func TestCancelSubscriptionEmptyFeedback(t *testing.T) {
	t.Skip("TODO: Requires real Mobius/NMI test account to create subscriptions that can be cancelled")

	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products and create an NMI subscription
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create an active NMI subscription
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorMobius,
		ProcessorSubID: "test-mobius-sub-empty-" + t.Name(),
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
		Processor:      models.ProcessorMobius,
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

// TestCancelSubscriptionAuthBoundaries tests that users can only cancel their own subscriptions
// TODO: The "user B can cancel" subtest requires real Mobius test account - see progress.json "nmi-test-account-integration"
func TestCancelSubscriptionAuthBoundaries(t *testing.T) {
	suite := setupTestSuite(t)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create two different users with their own tokens (use UUIDs for user IDs)
	userAID := uuid.New().String()
	userBID := uuid.New().String()

	tokenA := getTestIssuer().CreateToken(userAID, "usera@test.com")
	_ = getTestIssuer().CreateToken(userBID, "userb@test.com") // tokenB unused until we have real NMI account

	// Create an active subscription for User B only
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userBID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorMobius,
		ProcessorSubID: "test-nmi-userb-" + uuid.New().String()[:8],
	})

	t.Run("user A cannot cancel user B subscription", func(t *testing.T) {
		// User A tries to cancel (but they have no subscription)
		body := map[string]string{"feedback": "trying to cancel someone else's sub"}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenA)

		suite.Server.Handler().ServeHTTP(w, req)

		// User A should get "no active subscription" error since they don't have one
		// The endpoint only cancels the authenticated user's subscription
		assert.Equal(t, http.StatusInternalServerError, w.Code, "User A should not be able to cancel")

		// Verify User B's subscription is still active
		ctx := context.Background()
		var status string
		err := suite.BunDB.NewSelect().
			TableExpr("billing.subscriptions").
			Column("status").
			Where("user_id = ?", userBID).
			Limit(1).
			Scan(ctx, &status)
		require.NoError(t, err)
		assert.Equal(t, "active", status, "User B's subscription should still be active")
	})

	// TODO: Requires real Mobius/NMI test account - see progress.json "nmi-test-account-integration"
	// t.Run("user B can cancel their own subscription", func(t *testing.T) {
	// 	body := map[string]string{"feedback": "cancelling my own sub"}
	// 	jsonBody, _ := json.Marshal(body)
	//
	// 	w := httptest.NewRecorder()
	// 	req, _ := http.NewRequest("POST", "/v1/subscriptions/cancel", bytes.NewReader(jsonBody))
	// 	req.Header.Set("Content-Type", "application/json")
	// 	req.Header.Set("Authorization", "Bearer "+tokenB)
	//
	// 	suite.Server.Handler().ServeHTTP(w, req)
	//
	// 	// User B should successfully cancel their own subscription
	// 	assert.Equal(t, http.StatusOK, w.Code, "User B should be able to cancel their own subscription")
	//
	// 	// Verify User B's subscription is now cancelled
	// 	ctx := context.Background()
	// 	var status string
	// 	err := suite.BunDB.NewSelect().
	// 		TableExpr("billing.subscriptions").
	// 		Column("status").
	// 		Where("user_id = ?", userBID).
	// 		Limit(1).
	// 		Scan(ctx, &status)
	// 	require.NoError(t, err)
	// 	assert.Equal(t, "cancelled", status, "User B's subscription should now be cancelled")
	// })
}

// TestAdminCancelSubscription tests admin cancel endpoints
// TODO: Requires real Mobius test account - see progress.json "nmi-test-account-integration"
func TestAdminCancelSubscription(t *testing.T) {
	t.Skip("TODO: Requires real Mobius/NMI test account to create subscriptions that can be cancelled")

	suite, adminToken := setupAdminTestSuite(t)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	t.Run("admin can cancel any user subscription by subscription ID", func(t *testing.T) {
		// Create a subscription for a random user
		userID := uuid.New().String()
		sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:         userID,
			PriceID:        priceID,
			Status:         models.StatusActive,
			Processor:      models.ProcessorMobius,
			ProcessorSubID: "test-admin-cancel-1-" + uuid.New().String()[:8],
		})

		// Admin cancels the subscription by ID
		body := map[string]string{"reason": "Admin cancelled for testing"}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/subscriptions/"+sub.ID.String()+"/cancel", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Admin should be able to cancel subscription, got: %s", w.Body.String())

		// Verify subscription is cancelled
		ctx := context.Background()
		var status string
		err := suite.BunDB.NewSelect().
			TableExpr("billing.subscriptions").
			Column("status").
			Where("id = ?", sub.ID).
			Scan(ctx, &status)
		require.NoError(t, err)
		assert.Equal(t, "cancelled", status, "Subscription should be cancelled")
	})
}
