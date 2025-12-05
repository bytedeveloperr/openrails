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

// TestSolanaPayRequiresAuth tests that /v1/solana/pay endpoints require auth
func TestSolanaPayRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("POST /v1/solana/pay returns 401 without auth", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "price_22222222-2222-2222-2222-222222222222",
			"token":    "SOL",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/pay", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("GET /v1/solana/pay/:reference returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/solana/pay/test-reference", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestSolanaPayValidation tests request validation for POST /v1/solana/pay
func TestSolanaPayValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns 400 for missing price_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"token": "SOL",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/pay", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for missing token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "price_22222222-2222-2222-2222-222222222222",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/pay", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for invalid price_id format", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "not-a-valid-format",
			"token":    "SOL",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/pay", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})
}

// TestSolanaPayGeneratesURL tests that POST /v1/solana/pay generates a valid Solana Pay URL
func TestSolanaPayGeneratesURL(t *testing.T) {
	suite, token, _ := setupTestSuiteWithSolana(t)

	// Seed products
	products := suite.SeedProducts()
	priceID := "price_" + products[0].Prices[0].ID.String()

	t.Run("generates Solana Pay URL for valid request", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": priceID,
			"token":    "SOL",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/pay", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response struct {
			URL         string `json:"url"`
			Reference   string `json:"reference"`
			Amount      int64  `json:"amount"`
			Currency    string `json:"currency"`
			TokenAmount string `json:"token_amount"`
			Token       string `json:"token"`
			ExpiresAt   int64  `json:"expires_at"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response.URL, "solana:", "URL should start with solana: scheme")
		assert.NotEmpty(t, response.Reference, "Reference should not be empty")
		assert.Equal(t, "SOL", response.Token, "Token should be SOL")
		assert.Equal(t, "usd", response.Currency, "Currency should be usd")
		assert.Greater(t, response.Amount, int64(0), "Amount should be positive")
		assert.Greater(t, response.ExpiresAt, int64(0), "ExpiresAt should be set")
		assert.NotEmpty(t, response.TokenAmount, "TokenAmount should be set")
	})
}

// TestSolanaPayByReferenceEndpoint tests GET /v1/solana/pay/:reference endpoint
func TestSolanaPayByReferenceEndpoint(t *testing.T) {
	suite, token, _ := setupTestSuiteWithSolana(t)

	t.Run("returns expired for non-existent reference", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/solana/pay/non-existent-reference", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response struct {
			Status    string  `json:"status"`
			PaymentID *string `json:"payment_id,omitempty"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "expired", response.Status, "Status should be expired for non-existent reference")
	})
}

// TestSolanaPayFullFlow tests the full Solana Pay flow: create -> poll -> (simulated confirm)
func TestSolanaPayFullFlow(t *testing.T) {
	suite, token, _ := setupTestSuiteWithSolana(t)

	// Seed products
	products := suite.SeedProducts()
	priceID := "price_" + products[0].Prices[0].ID.String()

	var reference string

	t.Run("step 1: create payment request", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": priceID,
			"token":    "USDC",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/pay", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response struct {
			URL         string `json:"url"`
			Reference   string `json:"reference"`
			Amount      int64  `json:"amount"`
			TokenAmount string `json:"token_amount"`
			Token       string `json:"token"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		reference = response.Reference
		assert.NotEmpty(t, reference, "Reference should be set")
		assert.Equal(t, "USDC", response.Token, "Token should be USDC")
		assert.Contains(t, response.URL, "reference="+reference, "URL should contain reference")
	})

	t.Run("step 2: check payment status (should be pending)", func(t *testing.T) {
		require.NotEmpty(t, reference, "Reference should be set from step 1")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/solana/pay/"+reference, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response struct {
			Status    string  `json:"status"`
			PaymentID *string `json:"payment_id,omitempty"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "pending", response.Status, "Status should be pending")
	})
}

// TestSolanaPayInvalidToken tests invalid token handling
func TestSolanaPayInvalidToken(t *testing.T) {
	suite, token, _ := setupTestSuiteWithSolana(t)

	// Seed products
	products := suite.SeedProducts()
	priceID := "price_" + products[0].Prices[0].ID.String()

	t.Run("returns 400 for unsupported token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": priceID,
			"token":    "INVALID_TOKEN",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/pay", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for invalid token")
	})
}

// TestSolanaPayNonExistentPrice tests handling of non-existent price
func TestSolanaPayNonExistentPrice(t *testing.T) {
	suite, token, _ := setupTestSuiteWithSolana(t)

	t.Run("returns 400 for non-existent price", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "price_00000000-0000-0000-0000-000000000000",
			"token":    "SOL",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/solana/pay", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for non-existent price")
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
