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

// TestSolanaGenerateRequiresAuth tests that generate endpoint requires authentication
func TestSolanaGenerateRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":    "22222222-2222-2222-2222-222222222222",
			"token":       "SOL",
			"user_wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/generate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("returns 401 with invalid token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":    "22222222-2222-2222-2222-222222222222",
			"token":       "SOL",
			"user_wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/generate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer invalid-token")
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestSolanaGenerateValidation tests request validation for generate endpoint
func TestSolanaGenerateValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products
	suite.SeedProducts()

	t.Run("returns 400 for missing price_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"token":       "SOL",
			"user_wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/generate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for invalid price_id format", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":    "not-a-uuid",
			"token":       "SOL",
			"user_wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/generate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for missing token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":    "22222222-2222-2222-2222-222222222222",
			"user_wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/generate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns error for missing user_wallet", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "22222222-2222-2222-2222-222222222222",
			"token":    "SOL",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/generate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		// Handler returns 500 for empty wallet (internal validation error)
		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 for empty wallet")
	})
}

// TestSolanaGenerateWalletNotLinked tests error when wallet is not linked to user
func TestSolanaGenerateWalletNotLinked(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID.String()

	t.Run("returns 400 for wallet not linked to user", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":    priceID,
			"token":       "SOL",
			"user_wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/generate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for unlinked wallet")
	})
}

// TestSolanaQRRequiresAuth tests that QR endpoint requires authentication
func TestSolanaQRRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":    "22222222-2222-2222-2222-222222222222",
			"token":       "SOL",
			"user_wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/qr", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestSolanaQRValidation tests request validation for QR endpoint
func TestSolanaQRValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products
	suite.SeedProducts()

	t.Run("returns 400 for missing price_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"token":       "SOL",
			"user_wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/qr", bytes.NewReader(body))
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
			"price_id":    priceID,
			"token":       "INVALID_TOKEN",
			"user_wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/qr", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid token")
	})
}

// TestSolanaCheckRequiresAuth tests that check endpoint requires authentication
func TestSolanaCheckRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/solana/check?reference=test-reference", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestSolanaCheckValidation tests request validation for check endpoint
func TestSolanaCheckValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns 400 for missing reference", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/solana/check", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns pending for non-existent reference", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/solana/check?reference=nonexistent-reference-key", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// Should return 200 with "pending" status (no intent found)
		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK for non-existent reference")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "pending", response["status"], "Should return pending status")
	})
}

// TestSolanaSubmitRequiresAuth tests that submit endpoint requires authentication
func TestSolanaSubmitRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"signed_transaction": "base64-encoded-transaction",
			"price_id":           "22222222-2222-2222-2222-222222222222",
			"intent_id":          "33333333-3333-3333-3333-333333333333",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/submit", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestSolanaSubmitValidation tests request validation for submit endpoint
func TestSolanaSubmitValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns 400 for missing signed_transaction", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":  "22222222-2222-2222-2222-222222222222",
			"intent_id": "33333333-3333-3333-3333-333333333333",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/submit", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for missing price_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"signed_transaction": "base64-encoded-transaction",
			"intent_id":          "33333333-3333-3333-3333-333333333333",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/submit", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for missing intent_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"signed_transaction": "base64-encoded-transaction",
			"price_id":           "22222222-2222-2222-2222-222222222222",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/submit", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for invalid intent_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"signed_transaction": "base64-encoded-transaction",
			"price_id":           "22222222-2222-2222-2222-222222222222",
			"intent_id":          "33333333-3333-3333-3333-333333333333",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/submit", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		// Will return 400 because intent doesn't exist
		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for non-existent intent")
	})
}

// TestSolanaQRGeneratesURL tests that QR endpoint generates a valid Solana Pay URL
func TestSolanaQRGeneratesURL(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID.String()

	t.Run("generates Solana Pay URL for valid request", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":    priceID,
			"token":       "SOL",
			"user_wallet": "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/qr", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response struct {
			URL         string `json:"url"`
			Amount      int64  `json:"amount"` // Amount in cents
			TokenAmount string `json:"token_amount"`
			TokenSymbol string `json:"token_symbol"`
			Label       string `json:"label"`
			Message     string `json:"message"`
			ExpiresAt   int64  `json:"expires_at"`
			Reference   string `json:"reference"`
			IntentID    string `json:"intent_id"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify Solana Pay URL format
		assert.Contains(t, response.URL, "solana:", "URL should start with solana: scheme")
		assert.NotEmpty(t, response.Reference, "Reference should not be empty")
		assert.NotEmpty(t, response.IntentID, "IntentID should not be empty")
		assert.Equal(t, "SOL", response.TokenSymbol, "Token symbol should be SOL")
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
