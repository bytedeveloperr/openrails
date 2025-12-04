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
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
)

// TestCCBillFlexFormRequiresAuth tests that FlexForm endpoint requires authentication
func TestCCBillFlexFormRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "22222222-2222-2222-2222-222222222222",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("returns 401 with invalid token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id": "22222222-2222-2222-2222-222222222222",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer invalid-token")
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestCCBillFlexFormValidation tests request validation for FlexForm endpoint
func TestCCBillFlexFormValidation(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products so we have valid price IDs
	suite.SeedProducts()

	t.Run("returns 400 for missing price_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"first_name": "John",
			"last_name":  "Doe",
			"address1":   "123 Test St",
			"city":       "Testville",
			"state":      "TS",
			"zip_code":   "12345",
			"country":    "US",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request for missing price_id")
	})

	t.Run("returns 400 for invalid price_id format", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":   "not-a-uuid",
			"first_name": "John",
			"last_name":  "Doe",
			"address1":   "123 Test St",
			"city":       "Testville",
			"state":      "TS",
			"zip_code":   "12345",
			"country":    "US",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request for invalid price_id")
	})

	t.Run("returns 400 for non-existent price_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			"first_name": "John",
			"last_name":  "Doe",
			"address1":   "123 Test St",
			"city":       "Testville",
			"state":      "TS",
			"zip_code":   "12345",
			"country":    "US",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request for non-existent price_id")
	})

	// Note: The endpoint currently allows empty customer fields and uses them as-is in the URL.
	// This is acceptable as CCBill will validate the form fields on their side.
	// We test that valid requests work rather than testing validation of optional fields.
}

// TestCCBillFlexFormSuccess tests successful FlexForm URL generation
func TestCCBillFlexFormSuccess(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products with CCBill configuration
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID.String()

	t.Run("generates FlexForm URL successfully", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":   priceID,
			"first_name": "John",
			"last_name":  "Doe",
			"address1":   "123 Test Street",
			"city":       "Los Angeles",
			"state":      "CA",
			"zip_code":   "90001",
			"country":    "US",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response ccbill.FlexFormResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify response contains expected fields
		assert.NotEmpty(t, response.IFrameURL, "IFrameURL should not be empty")
		assert.NotEmpty(t, response.Width, "Width should not be empty")
		assert.NotEmpty(t, response.Height, "Height should not be empty")
	})

	t.Run("URL contains correct parameters", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":   priceID,
			"first_name": "Jane",
			"last_name":  "Smith",
			"address1":   "456 Oak Avenue",
			"city":       "New York",
			"state":      "NY",
			"zip_code":   "10001",
			"country":    "US",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response ccbill.FlexFormResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify URL contains required CCBill parameters
		assert.Contains(t, response.IFrameURL, "clientAccnum=", "URL should contain client account number")
		assert.Contains(t, response.IFrameURL, "clientSubacc=", "URL should contain client sub account")
		assert.Contains(t, response.IFrameURL, "formName=", "URL should contain form name")
		assert.Contains(t, response.IFrameURL, "customer_fname=Jane", "URL should contain first name")
		assert.Contains(t, response.IFrameURL, "customer_lname=Smith", "URL should contain last name")
		assert.Contains(t, response.IFrameURL, "city=New", "URL should contain city")
		assert.Contains(t, response.IFrameURL, "state=NY", "URL should contain state")
		assert.Contains(t, response.IFrameURL, "country=US", "URL should contain country")
		assert.Contains(t, response.IFrameURL, "username=", "URL should contain username (CCBill alias)")
		assert.Contains(t, response.IFrameURL, "signature=", "URL should contain signature")
	})
}

// TestCCBillFlexFormCreatesAlias tests that FlexForm request creates a CCBill username alias
func TestCCBillFlexFormCreatesAlias(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products with CCBill configuration
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID.String()

	t.Run("creates CCBill alias for user", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":   priceID,
			"first_name": "Test",
			"last_name":  "User",
			"address1":   "789 Main Blvd",
			"city":       "Chicago",
			"state":      "IL",
			"zip_code":   "60601",
			"country":    "US",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		// Verify alias was created in database
		ctx := suite.ctx
		var alias string
		var aliasUserID string
		err := suite.BunDB.NewSelect().
			TableExpr("billing.ccbill_username_aliases").
			Column("alias", "user_id").
			Where("user_id = ?", userID).
			Limit(1).
			Scan(ctx, &alias, &aliasUserID)
		require.NoError(t, err, "Should find CCBill alias for user")

		assert.NotEmpty(t, alias, "Alias should not be empty")
		assert.Equal(t, userID, aliasUserID, "Alias should map to correct user")

		// Verify the alias is in the generated URL
		var response ccbill.FlexFormResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response.IFrameURL, "username="+alias, "URL should contain the user's alias")
	})
}

// TestCCBillFlexFormPriceWithoutCCBill tests error when price isn't configured for CCBill
func TestCCBillFlexFormPriceWithoutCCBill(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	// Seed products so we have a valid product
	ctx := suite.ctx
	products := suite.SeedProducts()
	product := products[0].Product
	now := time.Now()

	// Create a new price without CCBillPriceID using the models.Price type
	billingCycleDays := 30
	priceWithoutCCBill := &models.Price{
		ID:               uuid.MustParse("99999999-9999-9999-9999-999999999999"),
		ProductID:        product.ID,
		DisplayName:      "No CCBill Price",
		Amount:           4.99,
		Currency:         "USD",
		BillingCycleDays: &billingCycleDays,
		IsActive:         true,
		// CCBillPriceID intentionally nil
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := suite.BunDB.NewInsert().Model(priceWithoutCCBill).
		On("CONFLICT (id) DO UPDATE").
		Set("display_name = EXCLUDED.display_name").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	require.NoError(t, err, "Failed to create price without CCBill")

	t.Run("returns 400 for price without CCBill configuration", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"price_id":   "99999999-9999-9999-9999-999999999999",
			"first_name": "John",
			"last_name":  "Doe",
			"address1":   "123 Test St",
			"city":       "Testville",
			"state":      "TS",
			"zip_code":   "12345",
			"country":    "US",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/subscriptions/ccbill/flexform-url", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 for price without CCBill config, got: %s", w.Body.String())
	})
}
