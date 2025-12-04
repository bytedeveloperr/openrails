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

	"github.com/doujins-org/doujins-billing/config"
)

// TestSolanaTokensNoAuth tests that /v1/solana/tokens doesn't require auth
func TestSolanaTokensNoAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns supported tokens without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/solana/tokens", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response struct {
			Tokens []struct {
				Symbol   string  `json:"symbol"`
				Name     string  `json:"name"`
				Mint     string  `json:"mint"`
				Decimals int     `json:"decimals"`
				Price    float64 `json:"price"`
			} `json:"tokens"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Should have at least SOL and USDC
		assert.GreaterOrEqual(t, len(response.Tokens), 2, "Should have at least 2 tokens")

		// Find SOL token
		var foundSOL bool
		for _, token := range response.Tokens {
			if token.Symbol == "SOL" {
				foundSOL = true
				assert.Equal(t, "Solana", token.Name)
				assert.Equal(t, 9, token.Decimals)
				assert.NotEmpty(t, token.Mint)
			}
		}
		assert.True(t, foundSOL, "Should have SOL token")
	})
}

// TestPaymentIntentRequiresAuth tests that payment-intents endpoint requires authentication
func TestPaymentIntentRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "22222222-2222-2222-2222-222222222222",
			"token":    "SOL",
			"wallet":   "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("returns 401 with invalid token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "22222222-2222-2222-2222-222222222222",
			"token":    "SOL",
			"wallet":   "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer invalid-token")
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestPaymentIntentValidation tests request validation for payment-intents endpoint
func TestPaymentIntentValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products
	suite.SeedProducts()

	t.Run("returns 400 for missing price_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"token":  "SOL",
			"wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for invalid price_id format", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "not-a-uuid",
			"token":    "SOL",
			"wallet":   "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for missing token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "22222222-2222-2222-2222-222222222222",
			"wallet":   "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns error for missing wallet", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "22222222-2222-2222-2222-222222222222",
			"token":    "SOL",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		// Handler returns 500 for empty wallet (internal validation error)
		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 for empty wallet")
	})
}

// TestPaymentIntentWalletNotLinked tests error when wallet is not linked to user
func TestPaymentIntentWalletNotLinked(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID.String()

	t.Run("returns 400 for wallet not linked to user", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": priceID,
			"token":    "SOL",
			"wallet":   "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for unlinked wallet")
	})
}

// TestPaymentIntentQRRequiresAuth tests that payment-intents/qr endpoint requires authentication
func TestPaymentIntentQRRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "22222222-2222-2222-2222-222222222222",
			"token":    "SOL",
			"wallet":   "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents/qr", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestPaymentIntentQRValidation tests request validation for payment-intents/qr endpoint
func TestPaymentIntentQRValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products
	suite.SeedProducts()

	t.Run("returns 400 for missing price_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"token":  "SOL",
			"wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents/qr", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for invalid token type", func(t *testing.T) {
		// First seed products to get a valid price ID
		products := suite.SeedProducts()
		priceID := products[0].Prices[0].ID.String()

		body, _ := json.Marshal(map[string]string{
			"price_id": priceID,
			"token":    "INVALID_TOKEN",
			"wallet":   "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents/qr", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid token")
	})
}

// TestGetPaymentIntentRequiresAuth tests that GET payment-intents/:id requires authentication
func TestGetPaymentIntentRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/payment-intents/pi_22222222-2222-2222-2222-222222222222", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestGetPaymentIntentValidation tests request validation for GET payment-intents/:id
func TestGetPaymentIntentValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns 400 for invalid intent ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/payment-intents/not-a-uuid", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 404 for non-existent intent", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/payment-intents/pi_22222222-2222-2222-2222-222222222222", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should return 404 for non-existent intent
		assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 for non-existent intent")
	})
}

// TestConfirmPaymentIntentRequiresAuth tests that confirm endpoint requires authentication
func TestConfirmPaymentIntentRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"signed_transaction": "base64-encoded-transaction",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents/pi_33333333-3333-3333-3333-333333333333/confirm", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestConfirmPaymentIntentValidation tests request validation for confirm endpoint
func TestConfirmPaymentIntentValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns 400 for missing signed_transaction", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents/pi_33333333-3333-3333-3333-333333333333/confirm", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for invalid intent_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"signed_transaction": "base64-encoded-transaction",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents/not-a-uuid/confirm", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for non-existent intent", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"signed_transaction": "base64-encoded-transaction",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents/pi_33333333-3333-3333-3333-333333333333/confirm", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		// Will return 400 because intent doesn't exist
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for non-existent intent")
	})
}

// TestPaymentIntentQRGeneratesURL tests that payment-intents/qr endpoint generates a valid Solana Pay URL
func TestPaymentIntentQRGeneratesURL(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID.String()

	t.Run("generates Solana Pay URL for valid request", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": priceID,
			"token":    "SOL",
			"wallet":   "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-intents/qr", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response struct {
			ID            string `json:"id"`
			Object        string `json:"object"`
			Status        string `json:"status"`
			Amount        int64  `json:"amount"`
			Currency      string `json:"currency"`
			PaymentMethod struct {
				Type        string `json:"type"`
				Token       string `json:"token"`
				TokenAmount string `json:"token_amount"`
			} `json:"payment_method"`
			Transaction struct {
				URL       string `json:"url"`
				Reference string `json:"reference"`
			} `json:"transaction"`
			ExpiresAt int64 `json:"expires_at"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify PaymentIntentObject format
		assert.Equal(t, "payment_intent", response.Object, "Should have object: payment_intent")
		assert.Contains(t, response.ID, "pi_", "ID should have pi_ prefix")
		assert.Contains(t, response.Transaction.URL, "solana:", "URL should start with solana: scheme")
		assert.NotEmpty(t, response.Transaction.Reference, "Reference should not be empty")
		assert.Equal(t, "SOL", response.PaymentMethod.Token, "Token should be SOL")
		assert.Greater(t, response.ExpiresAt, int64(0), "ExpiresAt should be set")
	})
}

// setupTestSuiteWithSolana configures the test suite with Solana config
func setupTestSuiteWithSolana(t *testing.T) (*TestContainerSuite, string, string) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Add Solana configuration
	suite.Config.Solana = &config.SolanaConfig{
		RPCEndpoint:               "",
		Network:                   "devnet",
		RecipientWallet:           "DzGLHdTfgHCYh8v3qNGJHn85CyX7aeFmqoUdVRBYkWMh",
		SupportedTokens:           config.DefaultDevnetTokens(),
		TransactionTimeoutSeconds: 30,
		ConfirmationBlocks:        1,
		MaxTransactionFee:         0.01,
	}

	return suite, token, userID
}
