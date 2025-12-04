//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/integrations/nmi"
)

// MockNMIServer simulates the NMI Direct Post API for testing
type MockNMIServer struct {
	Server            *httptest.Server
	RequestCount      int32
	LastRequest       map[string][]string
	ResponseOverride  string
	ShouldFail        bool
	FailReason        string
	VaultIDCounter    int32
	SubscriptionIDGen int32
}

// NewMockNMIServer creates a new mock NMI server
func NewMockNMIServer() *MockNMIServer {
	mock := &MockNMIServer{}
	mock.Server = httptest.NewServer(http.HandlerFunc(mock.handleRequest))
	return mock
}

func (m *MockNMIServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&m.RequestCount, 1)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	m.LastRequest = r.Form

	// Determine what type of request this is
	customerVault := r.Form.Get("customer_vault")
	recurring := r.Form.Get("recurring")

	var response string

	if m.ResponseOverride != "" {
		response = m.ResponseOverride
	} else if m.ShouldFail {
		failReason := m.FailReason
		if failReason == "" {
			failReason = "DECLINE"
		}
		response = fmt.Sprintf("response=2&responsetext=%s&response_code=300", failReason)
	} else if customerVault == "add_customer" {
		// Create customer vault response
		vaultID := fmt.Sprintf("vault_%d", atomic.AddInt32(&m.VaultIDCounter, 1))
		response = fmt.Sprintf("response=1&responsetext=SUCCESS&customer_vault_id=%s", vaultID)
	} else if customerVault == "update_customer" {
		response = "response=1&responsetext=SUCCESS"
	} else if customerVault == "delete_customer" {
		response = "response=1&responsetext=SUCCESS"
	} else if recurring == "add_subscription" {
		// Add subscription response
		subID := fmt.Sprintf("sub_%d", atomic.AddInt32(&m.SubscriptionIDGen, 1))
		txnID := fmt.Sprintf("txn_%d", atomic.AddInt32(&m.SubscriptionIDGen, 1))
		response = fmt.Sprintf("response=1&responsetext=SUCCESS&subscription_id=%s&transactionid=%s&authcode=123456&type=sale", subID, txnID)
	} else if recurring == "delete_subscription" {
		response = "response=1&responsetext=SUCCESS"
	} else if recurring == "rebill_subscription" {
		txnID := fmt.Sprintf("txn_%d", atomic.AddInt32(&m.SubscriptionIDGen, 1))
		response = fmt.Sprintf("response=1&responsetext=SUCCESS&transactionid=%s", txnID)
	} else {
		// Default sale response
		txnID := fmt.Sprintf("txn_%d", atomic.AddInt32(&m.SubscriptionIDGen, 1))
		response = fmt.Sprintf("response=1&responsetext=SUCCESS&transactionid=%s&authcode=123456&type=sale", txnID)
	}

	w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}

func (m *MockNMIServer) Close() {
	m.Server.Close()
}

func (m *MockNMIServer) URL() string {
	return m.Server.URL
}

func (m *MockNMIServer) Reset() {
	atomic.StoreInt32(&m.RequestCount, 0)
	m.LastRequest = nil
	m.ResponseOverride = ""
	m.ShouldFail = false
	m.FailReason = ""
}

// SetupSuiteWithMockNMI creates a test suite with mock NMI client configured
func SetupSuiteWithMockNMI(t *testing.T) (*TestContainerSuite, *MockNMIServer) {
	suite := setupTestSuite(t)
	mock := NewMockNMIServer()

	// Create NMI client with mock server URL
	nmiSettings := &config.NMIProviderSettings{
		Name:        "mobius",
		SecurityKey: "test-security-key",
		TestMode:    true,
	}

	client, err := nmi.NewClient("mobius", nmiSettings, false)
	require.NoError(t, err)

	// Override the DirectPostURL to point to mock server
	client.DirectPostURL = mock.URL()

	// Inject the mock client into the runtime
	suite.App.Runtime.NMIClients = map[string]*nmi.NMIClient{
		"mobius": client,
	}

	// Also update the subscription service's NMI clients
	if suite.App.Runtime.SubscriptionService != nil {
		suite.App.Runtime.SubscriptionService.NMIClients = suite.App.Runtime.NMIClients
	}
	if suite.App.Runtime.VaultService != nil {
		suite.App.Runtime.VaultService.NMIClients = suite.App.Runtime.NMIClients
	}

	t.Cleanup(func() {
		mock.Close()
	})

	return suite, mock
}

// TestSubscribeRequiresAuth tests that subscribe endpoint requires authentication
func TestSubscribeRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		body := map[string]string{
			"price_id":      uuid.New().String(),
			"gateway":       "mobius",
			"payment_token": "test-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/mobius", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("returns 401 with invalid token", func(t *testing.T) {
		body := map[string]string{
			"price_id":      uuid.New().String(),
			"gateway":       "mobius",
			"payment_token": "test-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/mobius", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer invalid-token")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestSubscribeNMISuccess tests successful NMI subscription creation
func TestSubscribeNMISuccess(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	// Seed products and prices
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create auth token for test user
	userID := uuid.New().String()
	email := "subscribe-test-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	t.Run("creates subscription with payment token", func(t *testing.T) {
		mock.Reset()

		body := map[string]interface{}{
			"price_id":      priceID.String(),
			"gateway":       "mobius",
			"provider":      "mobius",
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
		req, _ := http.NewRequest("POST", "/v1/subscriptions/mobius", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		// Verify NMI was called at least once (vault creation + subscription)
		assert.GreaterOrEqual(t, atomic.LoadInt32(&mock.RequestCount), int32(1), "Should have made NMI API calls")

		// Verify subscription was created in database
		subs := suite.GetAllSubscriptionsByUserID(userID)
		require.Len(t, subs, 1, "Should have one subscription")

		sub := subs[0]
		assert.Equal(t, models.StatusPending, sub.Status, "Subscription should be pending until webhook confirms")
		assert.Equal(t, models.ProcessorMobius, sub.Processor, "Processor should be mobius")
	})
}

// TestSubscribeNMIDeclined tests that declined payments are handled correctly
func TestSubscribeNMIDeclined(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	// Seed products and prices
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create auth token for test user
	userID := uuid.New().String()
	email := "subscribe-declined-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	t.Run("returns error when payment is declined", func(t *testing.T) {
		mock.Reset()
		mock.ShouldFail = true
		mock.FailReason = "DECLINE - Insufficient funds"

		body := map[string]interface{}{
			"price_id":      priceID.String(),
			"gateway":       "mobius",
			"provider":      "mobius",
			"payment_token": "test-payment-token-declined",
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
		req, _ := http.NewRequest("POST", "/v1/subscriptions/mobius", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should return error
		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 for declined payment")

		// Verify no subscription was created
		subs := suite.GetAllSubscriptionsByUserID(userID)
		assert.Empty(t, subs, "Should not have created subscription for declined payment")
	})
}

// TestSubscribeNMIWithExistingPaymentMethod tests subscribing with a saved payment method
func TestSubscribeNMIWithExistingPaymentMethod(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	// Seed products and prices
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create auth token for test user
	userID := uuid.New().String()
	email := "subscribe-pm-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	// Create a payment method for the user
	pm := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
		UserID:    userID,
		Processor: models.ProcessorMobius,
		VaultID:   "existing-vault-123",
		BillingID: "billing-123",
		IsActive:  true,
		LastFour:  "4242",
		CardType:  "Visa",
	})

	t.Run("creates subscription with existing payment method", func(t *testing.T) {
		mock.Reset()

		body := map[string]interface{}{
			"price_id":          priceID.String(),
			"gateway":           "mobius",
			"provider":          "mobius",
			"payment_method_id": pm.ID.String(),
			"email":             email,
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/mobius", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		// Verify subscription was created
		subs := suite.GetAllSubscriptionsByUserID(userID)
		require.Len(t, subs, 1, "Should have one subscription")

		sub := subs[0]
		assert.Equal(t, models.StatusPending, sub.Status)
		require.NotNil(t, sub.PaymentMethodID, "Should have payment method linked")
		assert.Equal(t, pm.ID, *sub.PaymentMethodID, "Should link to existing payment method")
	})
}

// TestSubscribeNMIInvalidPrice tests subscribing with invalid price ID
func TestSubscribeNMIInvalidPrice(t *testing.T) {
	suite, _ := SetupSuiteWithMockNMI(t)

	// Create auth token for test user
	userID := uuid.New().String()
	email := "subscribe-invalid-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	t.Run("returns error for non-existent price", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id":      uuid.New().String(), // Non-existent price
			"gateway":       "mobius",
			"payment_token": "test-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/mobius", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 for non-existent price")
	})

	t.Run("returns error for invalid price ID format", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id":      "not-a-uuid",
			"gateway":       "mobius",
			"payment_token": "test-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/mobius", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 for invalid price ID format")
	})
}

// TestSubscribeNMIAlreadySubscribed tests that users cannot subscribe twice
func TestSubscribeNMIAlreadySubscribed(t *testing.T) {
	suite, _ := SetupSuiteWithMockNMI(t)

	// Seed products and prices
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create auth token for test user
	userID := uuid.New().String()
	email := "subscribe-existing-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	// Create an existing active subscription
	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorMobius,
		ProcessorSubID: "existing-sub-123",
	})

	t.Run("returns error when user already has active subscription", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id":      priceID.String(),
			"gateway":       "mobius",
			"payment_token": "test-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/mobius", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 when already subscribed")
	})
}

// TestSubscribeCCBillRedirect tests that CCBill subscriptions redirect to FlexForm
func TestSubscribeCCBillRedirect(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products and prices
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	t.Run("returns redirect info for CCBill processor", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id":      priceID.String(),
			"gateway":       "ccbill",
			"payment_token": "test-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// CCBill should return redirect instructions
		assert.Equal(t, "redirect_required", response["status"], "Status should indicate redirect required")
		assert.Contains(t, response, "instructions", "Response should contain instructions")
		assert.Contains(t, response, "flexform_endpoint", "Response should contain flexform endpoint")
	})
}

// TestSubscribeNMIMissingPaymentInfo tests subscribing without payment token or method
func TestSubscribeNMIMissingPaymentInfo(t *testing.T) {
	suite, _ := SetupSuiteWithMockNMI(t)

	// Seed products and prices
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create auth token for test user
	userID := uuid.New().String()
	email := "subscribe-missing-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	t.Run("returns error without payment token or method", func(t *testing.T) {
		body := map[string]interface{}{
			"price_id": priceID.String(),
			"gateway":  "mobius",
			// No payment_token or payment_method_id
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/mobius", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 without payment info")
	})
}
