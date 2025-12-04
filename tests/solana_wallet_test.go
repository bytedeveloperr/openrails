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
)

// Test Solana wallet addresses (valid base58 format)
const (
	testSolanaWallet1 = "11111111111111111111111111111112"
	testSolanaWallet2 = "So11111111111111111111111111111111111111112"
	testSolanaWallet3 = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
)

// TestSolanaWalletRequiresAuth tests that wallet endpoints require authentication
func TestSolanaWalletRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("challenge returns 401 without auth", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"wallet": testSolanaWallet1})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/solana/challenge", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("verify returns 401 without auth", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"wallet":    testSolanaWallet1,
			"signature": "test-signature",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/solana/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("list returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/wallet/solana", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("linked returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/wallet/solana/linked", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("delete returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/v1/wallet/solana?wallet="+testSolanaWallet1, nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestSolanaWalletChallengeValidation tests input validation for challenge endpoint
func TestSolanaWalletChallengeValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns 400 for missing wallet", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/solana/challenge", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for invalid wallet address", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"wallet": "invalid-wallet",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/solana/challenge", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request for invalid wallet")
	})
}

// TestSolanaWalletChallengeSuccess tests successful challenge generation
func TestSolanaWalletChallengeSuccess(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("generates challenge for valid wallet", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"wallet": testSolanaWallet1,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/solana/challenge", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify challenge response contains expected fields
		assert.Contains(t, response, "message", "Response should contain message")
		assert.Contains(t, response, "expires_at", "Response should contain expires_at")
		assert.Contains(t, response, "wallet", "Response should contain wallet")
		assert.Contains(t, response, "nonce", "Response should contain nonce")
		assert.Equal(t, testSolanaWallet1, response["wallet"], "Wallet should match request")
		assert.NotEmpty(t, response["message"], "Message should not be empty")
		assert.NotEmpty(t, response["nonce"], "Nonce should not be empty")
	})
}

// TestSolanaWalletListEmpty tests listing wallets when none are linked
func TestSolanaWalletListEmpty(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns empty list for user with no wallets", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/wallet/solana", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		wallets, ok := response["wallets"].([]any)
		require.True(t, ok, "Response should contain wallets array")
		assert.Empty(t, wallets, "Wallets should be empty for new user")
	})
}

// TestSolanaWalletLinkedNotFound tests getting primary wallet when none exists
func TestSolanaWalletLinkedNotFound(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns 404 when no wallet linked", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/wallet/solana/linked", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 Not Found")
	})
}

// TestSolanaWalletDeleteValidation tests delete endpoint validation
func TestSolanaWalletDeleteValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns 400 for missing wallet param", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/v1/wallet/solana", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for invalid wallet address", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/v1/wallet/solana?wallet=invalid", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 500 for non-existent wallet", func(t *testing.T) {
		// Trying to delete a wallet that doesn't exist
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/v1/wallet/solana?wallet="+testSolanaWallet3, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		// The service returns error when wallet not found
		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should return 500 for non-existent wallet")
	})
}

// TestSolanaWalletVerifyValidation tests verify endpoint validation
func TestSolanaWalletVerifyValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns 400 for missing wallet", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"signature": "test-sig",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/solana/verify", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 400 for invalid wallet address", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"wallet":    "invalid-wallet",
			"signature": "test-sig",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/solana/verify", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})
}

// TestSolanaWalletLinkAndListFlow tests the full wallet linking flow
func TestSolanaWalletLinkAndListFlow(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("wallet appears in list after challenge", func(t *testing.T) {
		// Generate a challenge (this links the wallet in unverified state)
		body, _ := json.Marshal(map[string]string{
			"wallet": testSolanaWallet2,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/solana/challenge", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "Challenge should succeed")

		// Now list wallets
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/v1/wallet/solana", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "List should succeed")

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		wallets, ok := response["wallets"].([]any)
		require.True(t, ok, "Response should contain wallets array")
		require.Len(t, wallets, 1, "Should have one wallet")

		wallet := wallets[0].(map[string]any)
		assert.Equal(t, testSolanaWallet2, wallet["address"], "Wallet address should match")
		assert.False(t, wallet["is_verified"].(bool), "Wallet should not be verified yet")
	})
}

// TestSolanaWalletDeleteSuccess tests successful wallet deletion
func TestSolanaWalletDeleteSuccess(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Use a unique wallet address for this test to avoid conflicts with other tests
	// This wallet is unique to this test because the address is different
	deleteTestWallet := "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"

	// First, link a wallet via challenge
	body, _ := json.Marshal(map[string]string{
		"wallet": deleteTestWallet,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/wallet/solana/challenge", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	suite.Server.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "Challenge should succeed, got: %s", w.Body.String())

	t.Run("deletes wallet successfully", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/v1/wallet/solana?wallet="+deleteTestWallet, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Delete should succeed, got: %s", w.Body.String())

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, true, response["deleted"], "Deleted should be true")
		assert.Equal(t, deleteTestWallet, response["wallet"], "Wallet should match")
	})

	t.Run("wallet no longer in list after delete", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/wallet/solana", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "List should succeed")

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		wallets := response["wallets"].([]any)
		// Check that the deleted wallet is not in the list
		for _, w := range wallets {
			wallet := w.(map[string]any)
			assert.NotEqual(t, deleteTestWallet, wallet["address"], "Deleted wallet should not be in list")
		}
	})
}

// TestSolanaWalletDuplicateRejection tests that duplicate wallet linking is handled
func TestSolanaWalletDuplicateRejection(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	wallet := testSolanaWallet3

	t.Run("first challenge succeeds", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"wallet": wallet})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/solana/challenge", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "First challenge should succeed")
	})

	t.Run("second challenge for same wallet returns same wallet", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"wallet": wallet})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/wallet/solana/challenge", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		// The service should return success (it's idempotent - returns existing if already linked)
		require.Equal(t, http.StatusOK, w.Code, "Second challenge should also succeed (idempotent)")
	})

	t.Run("list shows only one wallet", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/wallet/solana", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		wallets := response["wallets"].([]any)
		// Count wallets with this address
		count := 0
		for _, w := range wallets {
			wmap := w.(map[string]any)
			if wmap["address"] == wallet {
				count++
			}
		}
		assert.Equal(t, 1, count, "Should have exactly one wallet with this address")
	})
}
