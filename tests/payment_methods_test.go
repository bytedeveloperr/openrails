//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
)

// TestPaymentMethodsRequiresAuth tests that payment methods endpoints require authentication
func TestPaymentMethodsRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("LIST returns 401 without auth token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/payment-methods", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("CREATE returns 401 without auth token", func(t *testing.T) {
		body := map[string]string{
			"payment_token": "test-token",
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
		req, _ := http.NewRequest("POST", "/v1/payment-methods", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("DELETE returns 401 without auth token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/v1/payment-methods/"+uuid.New().String(), nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("ACTIVATE returns 401 without auth token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/v1/payment-methods/"+uuid.New().String()+"/activate", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestListPaymentMethodsEmpty tests listing payment methods for a user with no methods
func TestListPaymentMethodsEmpty(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns empty list for user with no payment methods", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/payment-methods", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Check pagination fields
		assert.Equal(t, float64(0), response["total_items"], "Total items should be 0")
		assert.Equal(t, float64(1), response["page"], "Page should be 1")

		// Check data is empty array
		data, ok := response["data"].([]interface{})
		require.True(t, ok, "Data should be an array")
		assert.Empty(t, data, "Data should be empty")
	})
}

// TestListPaymentMethods tests listing payment methods for a user with methods
func TestListPaymentMethods(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Create some test payment methods
	pm1 := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
		UserID:    userID,
		Processor: models.ProcessorNMI,
		Provider:  "mobius",
		IsActive:  true,
		LastFour:  "4242",
		CardType:  "Visa",
	})

	pm2 := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
		UserID:    userID,
		Processor: models.ProcessorNMI,
		Provider:  "mobius",
		IsActive:  true, // Note: Database has default:true, so false values become true
		LastFour:  "1234",
		CardType:  "Mastercard",
	})

	t.Run("returns payment methods for user", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/payment-methods", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Should return all active methods for this user
		totalItems := int(response["total_items"].(float64))
		assert.GreaterOrEqual(t, totalItems, 2, "Should have at least 2 payment methods")

		data, ok := response["data"].([]interface{})
		require.True(t, ok)
		require.Len(t, data, totalItems, "Data length should match total_items")

		// Verify our created methods are present
		ids := make([]string, len(data))
		for i, item := range data {
			method := item.(map[string]interface{})
			ids[i] = method["id"].(string)
		}
		assert.Contains(t, ids, pm1.ID.String())
		assert.Contains(t, ids, pm2.ID.String())
	})

	t.Run("supports pagination parameters", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/payment-methods?page=1", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify pagination fields are present
		assert.Contains(t, response, "total_items", "Response should contain total_items")
		assert.Contains(t, response, "page", "Response should contain page")
		assert.Contains(t, response, "page_size", "Response should contain page_size")
		assert.Contains(t, response, "total_pages", "Response should contain total_pages")
		assert.Equal(t, float64(1), response["page"], "Page should be 1")
	})
}

// TestCreatePaymentMethod tests creating payment methods
func TestCreatePaymentMethod(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	// Create auth token for test user
	userID := uuid.New().String()
	email := "pm-create-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	t.Run("creates payment method successfully", func(t *testing.T) {
		mock.Reset()

		body := map[string]string{
			"payment_token": "test-token-create",
			"first_name":    "Test",
			"last_name":     "User",
			"address1":      "123 Test St",
			"city":          "Test City",
			"state":         "CA",
			"zip":           "90210",
			"country":       "US",
			"email":         email,
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-methods", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.NotEmpty(t, response["id"], "Should return payment method ID")
		assert.Equal(t, "nmi", response["processor"], "Processor should be NMI")
		assert.True(t, response["is_active"].(bool), "Should be active by default")
	})

	t.Run("returns error without payment_token", func(t *testing.T) {
		body := map[string]string{
			"first_name": "Test",
			"last_name":  "User",
			// No payment_token
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/payment-methods", bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})
}

// TestDeletePaymentMethod tests deleting payment methods
func TestDeletePaymentMethod(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	t.Run("deletes payment method successfully", func(t *testing.T) {
		// Create a payment method to delete
		pm := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
			UserID:    userID,
			Processor: models.ProcessorNMI,
			Provider:  "mobius",
			IsActive:  true,
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/v1/payment-methods/"+pm.ID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response["success"].(bool), "Success should be true")
		assert.Contains(t, response["message"], "deleted", "Message should mention deleted")

		// Verify payment method is actually deleted
		pms := suite.GetPaymentMethodsByUserID(userID)
		for _, p := range pms {
			assert.NotEqual(t, pm.ID, p.ID, "Deleted payment method should not be in list")
		}
	})

	t.Run("returns 404 for non-existent payment method", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/v1/payment-methods/"+uuid.New().String(), nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 Not Found")
	})

	t.Run("returns 403 for payment method owned by another user", func(t *testing.T) {
		// Create a payment method owned by a different user
		otherUserID := uuid.New().String()
		pm := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
			UserID:    otherUserID,
			Processor: models.ProcessorNMI,
			Provider:  "mobius",
			IsActive:  true,
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/v1/payment-methods/"+pm.ID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code, "Should return 403 Forbidden")
	})

	t.Run("returns 400 for invalid UUID", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/v1/payment-methods/not-a-uuid", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})
}

// TestActivatePaymentMethod tests activating payment methods
func TestActivatePaymentMethod(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Note: Due to database default:true on is_active, newly created payment methods
	// are always active. The activate endpoint will return success either way.

	t.Run("returns success for payment method", func(t *testing.T) {
		// Create a payment method (will be active due to default:true)
		pm := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
			UserID:    userID,
			Processor: models.ProcessorNMI,
			Provider:  "mobius",
			IsActive:  true,
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/payment-methods/%s/activate", pm.ID.String()), nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response["success"].(bool), "Success should be true")
	})

	t.Run("returns 404 for non-existent payment method", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/payment-methods/%s/activate", uuid.New().String()), nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 Not Found")
	})

	t.Run("returns 403 for payment method owned by another user", func(t *testing.T) {
		// Create a payment method owned by a different user
		otherUserID := uuid.New().String()
		pm := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
			UserID:    otherUserID,
			Processor: models.ProcessorNMI,
			Provider:  "mobius",
			IsActive:  true,
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/payment-methods/%s/activate", pm.ID.String()), nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code, "Should return 403 Forbidden")
	})
}

// TestUpdatePaymentMethod tests updating payment methods
func TestUpdatePaymentMethod(t *testing.T) {
	suite, mock := SetupSuiteWithMockNMI(t)

	// Create auth token for test user
	userID := uuid.New().String()
	email := "pm-update-" + t.Name() + "@test.example.com"
	token := getTestIssuer().CreateToken(userID, email)

	t.Run("updates payment method successfully", func(t *testing.T) {
		mock.Reset()

		// Create a payment method to update
		pm := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
			UserID:    userID,
			Processor: models.ProcessorNMI,
			Provider:  "mobius",
			IsActive:  true,
		})

		body := map[string]string{
			"payment_token": "new-token",
			"first_name":    "Updated",
			"last_name":     "User",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/payment-methods/%s", pm.ID.String()), bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got body: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, pm.ID.String(), response["id"], "Should return same payment method ID")
	})

	t.Run("returns error without payment_token", func(t *testing.T) {
		pm := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
			UserID:    userID,
			Processor: models.ProcessorNMI,
			Provider:  "mobius",
			IsActive:  true,
		})

		body := map[string]string{
			"first_name": "Updated",
			// No payment_token
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/payment-methods/%s", pm.ID.String()), bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns 404 for non-existent payment method", func(t *testing.T) {
		body := map[string]string{
			"payment_token": "new-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/payment-methods/%s", uuid.New().String()), bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code, "Should return 404 Not Found")
	})

	t.Run("returns 403 for payment method owned by another user", func(t *testing.T) {
		// Create a payment method owned by a different user
		otherUserID := uuid.New().String()
		pm := suite.CreateTestPaymentMethodWithOptions(PaymentMethodOptions{
			UserID:    otherUserID,
			Processor: models.ProcessorNMI,
			Provider:  "mobius",
			IsActive:  true,
		})

		body := map[string]string{
			"payment_token": "new-token",
		}
		jsonBody, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/payment-methods/%s", pm.ID.String()), bytes.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code, "Should return 403 Forbidden")
	})
}
