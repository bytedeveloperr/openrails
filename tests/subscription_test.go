//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/pkg/api"
)

// TestGetProductsEndpoint tests the public products endpoint returns seeded products
func TestGetProductsEndpoint(t *testing.T) {
	suite := setupTestSuite(t)

	// Seed products
	testProducts := suite.SeedProducts()
	require.Len(t, testProducts, 2, "Should have seeded 2 test products")

	t.Run("returns seeded products", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/products", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		// Parse list response with pagination
		var response api.ListResponse[api.ProductObject]
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err, "Should parse response JSON")

		// Verify list envelope
		assert.Equal(t, "list", response.Object, "Should have object: list")
		assert.GreaterOrEqual(t, response.TotalItems, int64(2), "Should have at least 2 total items")

		// Verify products returned (at least the seeded ones)
		require.GreaterOrEqual(t, len(response.Data), 2, "Should return at least 2 products")

		// Find premium-monthly product by name (Stripe uses name instead of slug)
		var monthlyProduct *api.ProductObject
		for i, p := range response.Data {
			if p.Name == "Premium Monthly" {
				monthlyProduct = &response.Data[i]
				break
			}
		}

		require.NotNil(t, monthlyProduct, "Should find Premium Monthly product")
		assert.Equal(t, "product", monthlyProduct.Object)
		assert.True(t, monthlyProduct.Active)
		require.Len(t, monthlyProduct.Prices, 1, "Should have 1 price")
		assert.Equal(t, int64(999), monthlyProduct.Prices[0].Amount, "Amount should be 999 cents")
		assert.Equal(t, "USD", monthlyProduct.Prices[0].Currency)
		assert.Equal(t, "price", monthlyProduct.Prices[0].Object)
	})

	t.Run("returns products with correct price details", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/products", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response api.ListResponse[api.ProductObject]
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Find yearly product and verify pricing
		var yearlyProduct *api.ProductObject
		for i, p := range response.Data {
			if p.Name == "Premium Yearly" {
				yearlyProduct = &response.Data[i]
				break
			}
		}

		require.NotNil(t, yearlyProduct, "Should find Premium Yearly product")
		require.Len(t, yearlyProduct.Prices, 1, "Should have 1 price")
		assert.Equal(t, int64(9999), yearlyProduct.Prices[0].Amount, "Amount should be 9999 cents")
		assert.NotNil(t, yearlyProduct.Prices[0].Recurring, "Should have recurring info")
		assert.Equal(t, "year", yearlyProduct.Prices[0].Recurring.Interval)
		assert.Equal(t, 1, yearlyProduct.Prices[0].Recurring.IntervalCount)
	})
}

// TestGetActiveSubscriptionEndpoint tests retrieving the current user's subscription
func TestGetActiveSubscriptionEndpoint(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products first
	testProducts := suite.SeedProducts()
	priceID := testProducts[0].Prices[0].ID

	t.Run("returns no subscription for new user", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/me/subscriptions?status=active", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// User without subscription should get 200 with empty list
		assert.Equal(t, http.StatusOK, w.Code)

		var response handlers.PaginatedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data, ok := response.Data.([]any)
		require.True(t, ok, "Data should be an array")
		assert.Empty(t, data, "Should have no active subscriptions for new user")
	})

	t.Run("returns active subscription details", func(t *testing.T) {
		// Create active subscription for user
		sub := suite.CreateTestSubscription(userID, priceID, models.StatusActive)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/me/subscriptions?status=active", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		// Parse paginated response
		var response handlers.PaginatedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Extract subscription data
		dataBytes, err := json.Marshal(response.Data)
		require.NoError(t, err)
		var subscriptions []services.UserSubscriptionResponse
		err = json.Unmarshal(dataBytes, &subscriptions)
		require.NoError(t, err)

		require.Len(t, subscriptions, 1, "Should have 1 active subscription")

		// Verify subscription data
		assert.Equal(t, sub.ID.String(), subscriptions[0].ID.String())
		assert.Equal(t, string(models.StatusActive), string(subscriptions[0].Status))
		assert.NotNil(t, subscriptions[0].Price, "Should include price details")
		assert.Equal(t, int64(999), subscriptions[0].Price.Amount, "Amount should be 999 cents")
	})

	t.Run("requires authentication", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/me/subscriptions", nil)
		// No auth header

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// TestGetSubscriptionHistoryEndpoint tests retrieving subscription history
func TestGetSubscriptionHistoryEndpoint(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products
	testProducts := suite.SeedProducts()
	monthlyPriceID := testProducts[0].Prices[0].ID
	yearlyPriceID := testProducts[1].Prices[0].ID

	t.Run("returns empty history for new user", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/me/subscriptions?status=all", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response handlers.PaginatedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data, ok := response.Data.([]any)
		require.True(t, ok, "Data should be an array")
		assert.Empty(t, data, "Should have no subscriptions for new user")
	})

	t.Run("returns subscription history with multiple subscriptions", func(t *testing.T) {
		// Create cancelled subscription
		cancelledSub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:  userID,
			PriceID: monthlyPriceID,
			Status:  models.StatusCancelled,
		})

		// Create active subscription
		activeSub := suite.CreateTestSubscription(userID, yearlyPriceID, models.StatusActive)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/me/subscriptions?status=all", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response handlers.PaginatedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Extract subscription data from paginated response
		dataBytes, err := json.Marshal(response.Data)
		require.NoError(t, err)
		var subscriptions []map[string]any
		err = json.Unmarshal(dataBytes, &subscriptions)
		require.NoError(t, err)
		require.Len(t, subscriptions, 2, "Should have 2 subscriptions in history")

		// Verify we have both active and cancelled subscriptions
		var hasActive, hasCancelled bool
		for _, sub := range subscriptions {
			status := sub["status"].(string)
			if status == string(models.StatusActive) {
				hasActive = true
				assert.Equal(t, activeSub.ID.String(), sub["id"])
			}
			if status == string(models.StatusCancelled) {
				hasCancelled = true
				assert.Equal(t, cancelledSub.ID.String(), sub["id"])
			}
		}
		assert.True(t, hasActive, "Should have active subscription")
		assert.True(t, hasCancelled, "Should have cancelled subscription")
	})
}

// TestGetUserPaymentsEndpoint tests retrieving payment history
func TestGetUserPaymentsEndpoint(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products
	testProducts := suite.SeedProducts()
	priceID := testProducts[0].Prices[0].ID

	t.Run("returns empty payments for new user", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/me/payments", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response handlers.PaginatedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data, ok := response.Data.([]any)
		require.True(t, ok, "Data should be an array")
		assert.Empty(t, data, "Should have no payments for new user")
	})

	t.Run("returns payment history", func(t *testing.T) {
		// Create subscription and payments
		sub := suite.CreateTestSubscription(userID, priceID, models.StatusActive)
		payment1 := suite.CreateTestPayment(userID, priceID, &sub.ID)
		payment2 := suite.CreateTestPayment(userID, priceID, &sub.ID)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/me/payments", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response handlers.PaginatedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Extract payment data from paginated response
		dataBytes, err := json.Marshal(response.Data)
		require.NoError(t, err)
		var payments []map[string]any
		err = json.Unmarshal(dataBytes, &payments)
		require.NoError(t, err)
		require.Len(t, payments, 2, "Should have 2 payments")

		// Verify payment details
		paymentIDs := make(map[string]bool)
		for _, p := range payments {
			paymentIDs[p["id"].(string)] = true
			// JSON unmarshals numbers as float64, but we compare against int64 value
			assert.Equal(t, float64(999), p["amount"], "Amount should be 999 cents")
			assert.Equal(t, "USD", p["currency"])
		}
		assert.True(t, paymentIDs[payment1.ID.String()], "Should include payment 1")
		assert.True(t, paymentIDs[payment2.ID.String()], "Should include payment 2")
	})
}

// TestGetMyBillingStatusEndpoint tests the user's billing status
func TestGetMyBillingStatusEndpoint(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products
	testProducts := suite.SeedProducts()
	priceID := testProducts[0].Prices[0].ID

	t.Run("returns non-premium status for user without subscription", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/me/status", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response handlers.BillingStatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Nil(t, response.Subscription, "Should have no subscription")
		assert.Nil(t, response.NextRenewalAt, "Should have no renewal date")

		// Check entitlements list is empty or nil
		if response.Entitlements != nil {
			ents, ok := response.Entitlements.([]interface{})
			if ok {
				assert.Empty(t, ents, "Should have no entitlements")
			}
		}
	})

	t.Run("returns premium status for user with active subscription", func(t *testing.T) {
		// Create active subscription
		sub := suite.CreateTestSubscription(userID, priceID, models.StatusActive)

		// Create entitlement
		suite.CreateTestEntitlement(userID, "premium", &sub.ID, models.EntitlementSourceSubscription)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/me/status", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response handlers.BillingStatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.NotNil(t, response.Subscription, "Should have active subscription")
		assert.NotNil(t, response.NextRenewalAt, "Should have renewal date")
		assert.NotNil(t, response.Entitlements, "Should have entitlements")
	})

	t.Run("requires authentication", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/me/status", nil)
		// No auth header

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// TestFlexFormURL tests the CCBill FlexForm URL generation
func TestFlexFormURL(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products
	testProducts := suite.SeedProducts()
	priceID := testProducts[0].Prices[0].ID

	t.Run("requires authentication", func(t *testing.T) {
		body := []byte(`{"price_id":"` + priceID.String() + `"}`)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", nil)
		req.Body = newRequestBody(body)
		req.Header.Set("Content-Type", "application/json")
		// No auth header

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("validates price_id parameter", func(t *testing.T) {
		body := []byte(`{"price_id":"invalid-uuid"}`)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", nil)
		req.Body = newRequestBody(body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should fail validation (400) or processing (500 in dev mode without CCBill config)
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusInternalServerError}, w.Code)
	})
}
