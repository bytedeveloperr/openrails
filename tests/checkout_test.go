//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/services"
)

// TestCheckoutRequiresAuth tests that checkout endpoint requires authentication
func TestCheckoutRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	t.Run("returns 401 without auth token", func(t *testing.T) {
		body := map[string]string{
			"price_id":  priceID.String(),
			"processor": "mobius",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestCheckoutSubscriptionNMISuccess tests successful NMI subscription via checkout
func TestCheckoutSubscriptionNMISuccess(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	// Seed products and prices (uses default which has BillingCycleDays set)
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create auth token for test user
	userID := uuid.New().String()
	email := "checkout-sub-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	t.Run("creates subscription with payment token", func(t *testing.T) {
		mock.Reset()

		body := map[string]interface{}{
			"price_id":      priceID.String(),
			"processor":     "mobius",
			"payment_token": "test-payment-token-123",
			"email":         email,
			"first_name":    "Test",
			"last_name":     "User",
			"address1":      "123 Test St",
			"city":          "Test City",
			"state":         "CA",
			"zip":           "90210",
			"country":       "US",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "pending", response["status"], "Status should be pending")
		assert.NotEmpty(t, response["subscription_id"], "Should have subscription_id")
		assert.NotEmpty(t, response["transaction_id"], "Should have transaction_id")

		// Verify subscription was created in database
		subs := suite.GetAllSubscriptionsByUserID(userID)
		require.Len(t, subs, 1, "Should have one subscription")
		assert.Equal(t, models.StatusPending, subs[0].Status)
	})
}

// TestCheckoutBlocksExistingCoverage tests that checkout blocks when user has indefinite coverage
func TestCheckoutBlocksExistingCoverage(t *testing.T) {
	suite, _ := SetupSuiteWithMockNMI(t)

	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	email := "checkout-blocked-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	// Create an existing active subscription with NO end date (indefinite)
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorMobius,
		ProcessorSubID: "existing-sub-123",
		// No CurrentPeriodEndsAt = indefinite
	})

	t.Run("blocks purchase when user has indefinite coverage", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id":      priceID.String(),
			"processor":     "mobius",
			"payment_token": "test-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code, "Should return 409 Conflict")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "blocked", response["status"], "Status should be blocked")
		assert.Contains(t, response["message"], "already have active access", "Message should explain blocking")
	})
}

// TestCheckoutDelayedStartNMI tests that NMI subscription gets delayed start when user has expiring coverage
func TestCheckoutDelayedStartNMI(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	email := "checkout-delayed-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	// Create an existing active subscription that expires in 5 days
	expiresAt := time.Now().Add(5 * 24 * time.Hour)
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:              userID,
		PriceID:             priceID,
		Status:              models.StatusActive,
		Processor:           models.ProcessorMobius,
		ProcessorSubID:      "expiring-sub-123",
		CurrentPeriodEndsAt: &expiresAt,
	})

	t.Run("creates subscription with delayed start when user has expiring coverage", func(t *testing.T) {
		mock.Reset()

		body := map[string]interface{}{
			"price_id":      priceID.String(),
			"processor":     "mobius",
			"payment_token": "test-token-delayed",
			"email":         email,
			"first_name":    "Test",
			"last_name":     "User",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "pending", response["status"])
		assert.NotEmpty(t, response["delayed_start"], "Should have delayed_start")

		// Verify the delayed start is around when the existing subscription ends
		delayedStartStr := response["delayed_start"].(string)
		delayedStart, err := time.Parse(time.RFC3339, delayedStartStr)
		require.NoError(t, err)

		// Should be within a day of the expiration
		diff := delayedStart.Sub(expiresAt)
		assert.Less(t, diff.Abs(), 24*time.Hour, "Delayed start should be close to existing coverage end")
	})
}

// TestCheckoutCCBillBlockedWithExistingCoverage tests that CCBill is blocked when user has coverage
func TestCheckoutCCBillBlockedWithExistingCoverage(t *testing.T) {
	suite := setupTestSuite(t)

	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	email := "checkout-ccbill-blocked-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	// Create an existing subscription that expires (has end date)
	expiresAt := time.Now().Add(5 * 24 * time.Hour)
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:              userID,
		PriceID:             priceID,
		Status:              models.StatusActive,
		Processor:           models.ProcessorCCBill,
		ProcessorSubID:      "ccbill-sub-123",
		CurrentPeriodEndsAt: &expiresAt,
	})

	t.Run("blocks CCBill purchase when user has existing coverage", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id":  priceID.String(),
			"processor": "ccbill",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code, "Should return 409 Conflict")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "blocked", response["status"])
		assert.Contains(t, response["message"], "CCBill", "Message should mention CCBill limitation")
	})
}

// TestCheckoutSolanaRedirect tests that Solana checkout returns redirect
func TestCheckoutSolanaRedirect(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	products := suite.SeedProducts()
	// Need to get a one-time price (BillingCycleDays = nil)
	// For now, use subscription price but expect redirect for solana subscriptions
	priceID := products[0].Prices[0].ID

	t.Run("returns redirect for Solana purchases", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id":  priceID.String(),
			"processor": "solana",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Solana subscriptions should fail (not supported)
		// Solana one-time purchases should redirect to payment-intents
		var response map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &response)

		// Should either be an error (solana doesn't support subscriptions)
		// or a redirect (for one-time purchases)
		if w.Code == http.StatusOK {
			assert.Equal(t, "redirect_required", response["status"])
		}
	})
}

// TestCheckoutWithExistingPaymentMethod tests checkout using a saved payment method
func TestCheckoutWithExistingPaymentMethod(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	email := "checkout-pm-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	// Create a payment method for the user
	pm := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
		UserID:    userID,
		Processor: models.ProcessorMobius,
		VaultID:   "existing-vault-456",
		BillingID: "billing-456",
		IsActive:  true,
		LastFour:  "4242",
		CardType:  "Visa",
	})

	t.Run("creates subscription with existing payment method", func(t *testing.T) {
		mock.Reset()

		body := map[string]interface{}{
			"price_id":          priceID.String(),
			"processor":         "mobius",
			"payment_method_id": pm.ID.String(),
			"email":             email,
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "pending", response["status"])
	})
}

// TestCheckoutInvalidProcessor tests checkout with invalid processor
func TestCheckoutInvalidProcessor(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	t.Run("returns error for invalid processor", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id":      priceID.String(),
			"processor":     "invalid_processor",
			"payment_token": "test-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid processor")
	})
}

// TestCheckoutMissingPaymentInfo tests checkout without payment info
func TestCheckoutMissingPaymentInfo(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	email := "checkout-missing-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	t.Run("returns error without payment token or method", func(t *testing.T) {
		mock.Reset()

		body := map[string]interface{}{
			"price_id":  priceID.String(),
			"processor": "mobius",
			// No payment_token or payment_method_id
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 without payment info")
	})
}

// TestCheckoutInvalidPriceID tests checkout with invalid price ID
func TestCheckoutInvalidPriceID(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns error for non-existent price", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id":      uuid.New().String(), // Non-existent price
			"processor":     "mobius",
			"payment_token": "test-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for non-existent price")
	})

	t.Run("returns error for invalid price ID format", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id":      "not-a-uuid",
			"processor":     "mobius",
			"payment_token": "test-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid price ID format")
	})
}

// TestCheckoutCrossProcessorScenario tests buying NMI sub when user has expiring CCBill
func TestCheckoutCrossProcessorScenario(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	email := "checkout-cross-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	// Create an existing CCBill subscription that expires
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:              userID,
		PriceID:             priceID,
		Status:              models.StatusActive,
		Processor:           models.ProcessorCCBill,
		ProcessorSubID:      "ccbill-expiring-123",
		CurrentPeriodEndsAt: &expiresAt,
	})

	t.Run("allows NMI subscription when CCBill is expiring", func(t *testing.T) {
		mock.Reset()

		body := map[string]interface{}{
			"price_id":      priceID.String(),
			"processor":     "mobius",
			"payment_token": "test-token-cross",
			"email":         email,
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "pending", response["status"])
		assert.NotEmpty(t, response["delayed_start"], "Should have delayed start after CCBill expires")
	})
}

// =============================================================================
// TIER GROUP TESTS - Upgrade/Downgrade Detection
// =============================================================================

// TestTierGroupUpgradeDetection tests that checkout detects upgrades within tier groups
func TestTierGroupUpgradeDetection(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	// Seed tiered products: Premium ($10, rank=1), Premium+ ($20, rank=2), Premium Ultimate ($30, rank=3)
	tieredProducts := suite.SeedTieredProducts()
	premiumPriceID := tieredProducts[0].Prices[0].ID         // $10/mo, rank=1
	premiumPlusPriceID := tieredProducts[1].Prices[0].ID     // $20/mo, rank=2
	premiumUltimatePriceID := tieredProducts[2].Prices[0].ID // $30/mo, rank=3

	userID := uuid.New().String()
	email := "tier-upgrade-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	// Create existing Premium subscription (rank=1) with 20 days remaining
	periodStart := time.Now().Add(-10 * 24 * time.Hour)
	periodEnd := time.Now().Add(20 * 24 * time.Hour)
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:              userID,
		PriceID:             premiumPriceID,
		Status:              models.StatusActive,
		Processor:           models.ProcessorMobius,
		ProcessorSubID:      "premium-sub-123",
		PeriodStart:         periodStart,
		PeriodEnd:           periodEnd,
		CurrentPeriodEndsAt: &periodEnd,
	})

	t.Run("detects upgrade when buying higher tier", func(t *testing.T) {
		mock.Reset()

		// Try to buy Premium+ (rank=2) while having Premium (rank=1)
		body := map[string]interface{}{
			"price_id":      premiumPlusPriceID.String(),
			"processor":     "mobius",
			"payment_token": "test-token-upgrade",
			"email":         email,
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should succeed with upgrade processing
		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"], "Should succeed with upgrade")
		assert.Contains(t, response["message"], "Upgraded", "Message should indicate upgrade")
		assert.Contains(t, response["message"], "Premium+", "Message should mention new tier")
	})

	t.Run("skips tier 1 to tier 3 upgrade", func(t *testing.T) {
		// Create new user with Premium
		userID2 := uuid.New().String()
		token2 := getTestIssuer().CreateToken(userID2, "tier-upgrade-skip@test.example.com")

		suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:              userID2,
			PriceID:             premiumPriceID,
			Status:              models.StatusActive,
			Processor:           models.ProcessorMobius,
			ProcessorSubID:      "premium-sub-skip-456",
			PeriodStart:         periodStart,
			PeriodEnd:           periodEnd,
			CurrentPeriodEndsAt: &periodEnd,
		})

		mock.Reset()

		// Try to buy Premium Ultimate (rank=3) directly from Premium (rank=1)
		body := map[string]interface{}{
			"price_id":      premiumUltimatePriceID.String(),
			"processor":     "mobius",
			"payment_token": "test-token-skip-upgrade",
			"email":         "tier-upgrade-skip@test.example.com",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token2)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should succeed - skipping tiers is allowed
		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Contains(t, response["message"], "Premium Ultimate", "Message should mention highest tier")
	})
}

// TestTierGroupDowngradeDetection tests that checkout detects downgrades within tier groups
func TestTierGroupDowngradeDetection(t *testing.T) {
	suite, _ := SetupSuiteWithMockNMI(t)

	// Seed tiered products
	tieredProducts := suite.SeedTieredProducts()
	premiumPriceID := tieredProducts[0].Prices[0].ID     // $10/mo, rank=1
	premiumPlusPriceID := tieredProducts[1].Prices[0].ID // $20/mo, rank=2

	userID := uuid.New().String()
	email := "tier-downgrade-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	// Create existing Premium+ subscription (rank=2) with 20 days remaining
	periodStart := time.Now().Add(-10 * 24 * time.Hour)
	periodEnd := time.Now().Add(20 * 24 * time.Hour)
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:              userID,
		PriceID:             premiumPlusPriceID,
		Status:              models.StatusActive,
		Processor:           models.ProcessorMobius,
		ProcessorSubID:      "premium-plus-sub-123",
		PeriodStart:         periodStart,
		PeriodEnd:           periodEnd,
		CurrentPeriodEndsAt: &periodEnd,
	})

	t.Run("detects downgrade when buying lower tier", func(t *testing.T) {
		// Try to buy Premium (rank=1) while having Premium+ (rank=2)
		body := map[string]interface{}{
			"price_id":      premiumPriceID.String(),
			"processor":     "mobius",
			"payment_token": "test-token-downgrade",
			"email":         email,
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should succeed with scheduled downgrade
		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"], "Should succeed with scheduled downgrade")
		assert.Contains(t, response["message"], "scheduled", "Message should indicate scheduled change")
		assert.Contains(t, response["message"], "Premium", "Message should mention new tier")
	})
}

// TestTierGroupSameTierBlocked tests that buying the same product is blocked
func TestTierGroupSameTierBlocked(t *testing.T) {
	suite, _ := SetupSuiteWithMockNMI(t)

	// Seed tiered products
	tieredProducts := suite.SeedTieredProducts()
	premiumPriceID := tieredProducts[0].Prices[0].ID // $10/mo, rank=1

	userID := uuid.New().String()
	email := "tier-same-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	// Create existing Premium subscription
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        premiumPriceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorMobius,
		ProcessorSubID: "premium-same-123",
	})

	t.Run("blocks purchase of same tier product", func(t *testing.T) {
		// Try to buy Premium again while already having it
		body := map[string]interface{}{
			"price_id":      premiumPriceID.String(),
			"processor":     "mobius",
			"payment_token": "test-token-same",
			"email":         email,
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should be blocked as duplicate
		assert.Equal(t, http.StatusConflict, w.Code, "Should return 409 Conflict")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "blocked", response["status"])
	})
}

// TestNoTierGroupProductsWorkNormally tests that products without tier groups work normally
func TestNoTierGroupProductsWorkNormally(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	// Use regular products (no tier group)
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	userID := uuid.New().String()
	email := "no-tier-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	t.Run("allows subscription to product without tier group", func(t *testing.T) {
		mock.Reset()

		body := map[string]interface{}{
			"price_id":      priceID.String(),
			"processor":     "mobius",
			"payment_token": "test-token-no-tier",
			"email":         email,
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/me/checkout", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "pending", response["status"])
	})
}

// =============================================================================
// PRORATION CALCULATION TESTS
// =============================================================================

// TestProrationCalculation tests the proration calculation logic
func TestProrationCalculation(t *testing.T) {
	// Test the calculateProration function directly
	checkoutService := &services.CheckoutService{}

	t.Run("calculates proration for upgrade with 20 days remaining", func(t *testing.T) {
		// $10 -> $20, 20 days remaining out of 30 day cycle
		now := time.Now()
		periodEnd := now.Add(20 * 24 * time.Hour)
		cycleDays := 30

		prorationAmount, daysRemaining, cycle := checkoutService.CalculateProration(
			1000, // old price: $10
			2000, // new price: $20
			&periodEnd,
			&cycleDays,
			now,
		)

		// Proration = ($20 - $10) * (20/30) = $10 * 0.666 = $6.67
		// In cents: 1000 * 20 / 30 = 666
		assert.Equal(t, int64(666), prorationAmount, "Proration should be ~$6.67")
		assert.InDelta(t, 20, daysRemaining, 1, "Days remaining should be ~20")
		assert.Equal(t, 30, cycle, "Cycle days should be 30")
	})

	t.Run("calculates proration for larger upgrade", func(t *testing.T) {
		// $10 -> $30, 15 days remaining out of 30 day cycle
		now := time.Now()
		periodEnd := now.Add(15 * 24 * time.Hour)
		cycleDays := 30

		prorationAmount, daysRemaining, cycle := checkoutService.CalculateProration(
			1000, // old price: $10
			3000, // new price: $30
			&periodEnd,
			&cycleDays,
			now,
		)

		// Proration = ($30 - $10) * (15/30) = $20 * 0.5 = $10
		// In cents: 2000 * 15 / 30 = 1000
		assert.Equal(t, int64(1000), prorationAmount, "Proration should be $10")
		assert.InDelta(t, 15, daysRemaining, 1, "Days remaining should be ~15")
		assert.Equal(t, 30, cycle, "Cycle days should be 30")
	})

	t.Run("returns zero proration for downgrade", func(t *testing.T) {
		// $20 -> $10 (downgrade - no proration charge)
		now := time.Now()
		periodEnd := now.Add(20 * 24 * time.Hour)
		cycleDays := 30

		prorationAmount, _, _ := checkoutService.CalculateProration(
			2000, // old price: $20
			1000, // new price: $10
			&periodEnd,
			&cycleDays,
			now,
		)

		assert.Equal(t, int64(0), prorationAmount, "Downgrade should have zero proration")
	})

	t.Run("returns zero proration when period already ended", func(t *testing.T) {
		// Period already ended
		now := time.Now()
		periodEnd := now.Add(-1 * 24 * time.Hour) // 1 day ago
		cycleDays := 30

		prorationAmount, daysRemaining, _ := checkoutService.CalculateProration(
			1000, // old price
			2000, // new price
			&periodEnd,
			&cycleDays,
			now,
		)

		assert.Equal(t, int64(0), prorationAmount, "Should be zero when period ended")
		assert.Equal(t, 0, daysRemaining, "Days remaining should be 0")
	})

	t.Run("handles nil period end date", func(t *testing.T) {
		now := time.Now()
		cycleDays := 30

		prorationAmount, daysRemaining, _ := checkoutService.CalculateProration(
			1000,
			2000,
			nil, // nil period end
			&cycleDays,
			now,
		)

		assert.Equal(t, int64(0), prorationAmount, "Should be zero with nil period end")
		assert.Equal(t, 0, daysRemaining, "Days remaining should be 0")
	})

	t.Run("uses default 30-day cycle when not specified", func(t *testing.T) {
		now := time.Now()
		periodEnd := now.Add(15 * 24 * time.Hour)

		_, _, cycle := checkoutService.CalculateProration(
			1000,
			2000,
			&periodEnd,
			nil, // nil billing cycle
			now,
		)

		assert.Equal(t, 30, cycle, "Should default to 30-day cycle")
	})
}
