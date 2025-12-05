//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/pkg/api"
)

// TestAdminGrantsRequireAuth verifies that grant endpoints require admin JWT
func TestAdminGrantsRequireAuth(t *testing.T) {
	suite, _ := setupAdminTestSuite(t)
	userID := uuid.New().String()

	t.Run("POST grants returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+userID+"/grants", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("GET grants returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/users/"+userID+"/grants", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("returns 403 with non-admin JWT", func(t *testing.T) {
		userToken := CreateUserToken(t, userID)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/users/"+userID+"/grants", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code, "Should return 403 Forbidden for non-admin user")
	})
}

// TestAdminGrantFreeComp tests granting a free comp (no payment)
func TestAdminGrantFreeComp(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	// Create a product and price for testing
	ctx := context.Background()
	product := &models.Product{
		ID:          uuid.New(),
		DisplayName: "Premium Access",
		Description: "Full premium access",
		IsActive:    true,
		EntitlementsSpec: map[string]*int{
			"premium_access": intPtr(30), // 30 days
		},
	}
	_, err := suite.BunDB.NewInsert().Model(product).Exec(ctx)
	require.NoError(t, err)

	price := &models.Price{
		ID:               uuid.New(),
		ProductID:        product.ID,
		DisplayName:      "Monthly Premium",
		Amount:           999,
		Currency:         "usd",
		BillingCycleDays: intPtr(30),
		IsActive:         true,
	}
	_, err = suite.BunDB.NewInsert().Model(price).Exec(ctx)
	require.NoError(t, err)

	userID := uuid.New().String()

	t.Run("creates free comp with default duration from product spec", func(t *testing.T) {
		body := fmt.Sprintf(`{
			"price_id": "%s",
			"reason": "Twitter giveaway winner"
		}`, api.FormatPriceID(price.ID))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+userID+"/grants", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "Should return 201 Created. Body: %s", w.Body.String())

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "admin_grant", resp["object"])
		assert.NotEmpty(t, resp["admin_grant_id"])
		assert.Nil(t, resp["payment_id"], "Free comp should not create payment")
		assert.Contains(t, resp["entitlements_granted"], "premium_access")
	})

	t.Run("creates free comp with custom duration override", func(t *testing.T) {
		anotherUserID := uuid.New().String()
		body := fmt.Sprintf(`{
			"price_id": "%s",
			"reason": "Bug compensation",
			"duration_days": 7
		}`, api.FormatPriceID(price.ID))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+anotherUserID+"/grants", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "Should return 201 Created. Body: %s", w.Body.String())

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "admin_grant", resp["object"])
		assert.Contains(t, resp["entitlements_granted"], "premium_access")

		// Verify the entitlement was created with 7 days duration
		var ent models.Entitlement
		err = suite.BunDB.NewSelect().
			Model(&ent).
			Where("ent.user_id = ?", anotherUserID).
			Where("ent.entitlement = ?", "premium_access").
			Scan(ctx)
		require.NoError(t, err)
		require.NotNil(t, ent.EndAt)
		// Should be approximately 7 days from now
		duration := ent.EndAt.Sub(ent.StartAt)
		assert.InDelta(t, 7*24, duration.Hours(), 1, "Should be ~7 days duration")
	})

	t.Run("creates indefinite access with duration_days=0", func(t *testing.T) {
		lifetimeUserID := uuid.New().String()
		body := fmt.Sprintf(`{
			"price_id": "%s",
			"reason": "Lifetime VIP",
			"duration_days": 0
		}`, api.FormatPriceID(price.ID))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+lifetimeUserID+"/grants", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "Should return 201 Created. Body: %s", w.Body.String())

		// Verify the entitlement was created with no end date (indefinite)
		var ent models.Entitlement
		err = suite.BunDB.NewSelect().
			Model(&ent).
			Where("ent.user_id = ?", lifetimeUserID).
			Where("ent.entitlement = ?", "premium_access").
			Scan(ctx)
		require.NoError(t, err)
		assert.Nil(t, ent.EndAt, "Indefinite grant should have nil EndAt")
	})
}

// TestAdminGrantWithPayment tests granting with a payment record (manual payment)
func TestAdminGrantWithPayment(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	// Create a product and price for testing
	ctx := context.Background()
	product := &models.Product{
		ID:          uuid.New(),
		DisplayName: "Premium Access",
		Description: "Full premium access",
		IsActive:    true,
		EntitlementsSpec: map[string]*int{
			"premium_access": intPtr(30),
		},
	}
	_, err := suite.BunDB.NewInsert().Model(product).Exec(ctx)
	require.NoError(t, err)

	price := &models.Price{
		ID:               uuid.New(),
		ProductID:        product.ID,
		DisplayName:      "Monthly Premium",
		Amount:           999,
		Currency:         "usd",
		BillingCycleDays: intPtr(30),
		IsActive:         true,
	}
	_, err = suite.BunDB.NewInsert().Model(price).Exec(ctx)
	require.NoError(t, err)

	userID := uuid.New().String()

	t.Run("creates payment record when amount > 0", func(t *testing.T) {
		body := fmt.Sprintf(`{
			"price_id": "%s",
			"reason": "PayPal payment",
			"amount": 999,
			"currency": "usd",
			"transaction_id": "PAYPAL-ABC123"
		}`, api.FormatPriceID(price.ID))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+userID+"/grants", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "Should return 201 Created. Body: %s", w.Body.String())

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "admin_grant", resp["object"])
		assert.NotEmpty(t, resp["admin_grant_id"])
		assert.NotNil(t, resp["payment_id"], "Manual payment should create payment record")
		assert.Contains(t, resp["entitlements_granted"], "premium_access")

		// Verify the payment was created
		var payment models.Payment
		paymentIDStr := resp["payment_id"].(string)
		paymentID, _, _ := api.TryParseID(paymentIDStr)
		err = suite.BunDB.NewSelect().
			Model(&payment).
			Where("purch.id = ?", paymentID).
			Scan(ctx)
		require.NoError(t, err)

		assert.Equal(t, int64(999), payment.Amount)
		assert.Equal(t, "usd", payment.Currency)
		assert.Equal(t, "PAYPAL-ABC123", payment.TransactionID)
		assert.Equal(t, models.ProcessorAdmin, payment.Processor)
	})

	t.Run("generates transaction_id if not provided", func(t *testing.T) {
		anotherUserID := uuid.New().String()
		body := fmt.Sprintf(`{
			"price_id": "%s",
			"reason": "Cash payment",
			"amount": 500
		}`, api.FormatPriceID(price.ID))

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+anotherUserID+"/grants", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// Verify payment has auto-generated transaction_id
		paymentIDStr := resp["payment_id"].(string)
		paymentID, _, _ := api.TryParseID(paymentIDStr)

		var payment models.Payment
		err = suite.BunDB.NewSelect().
			Model(&payment).
			Where("purch.id = ?", paymentID).
			Scan(ctx)
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(payment.TransactionID, "admin-grant-"), "Should generate admin-grant- prefixed transaction ID")
	})
}

// TestListAdminGrants tests GET /v1/admin/users/:user_id/grants
func TestListAdminGrants(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	// Create a product and price
	ctx := context.Background()
	product := &models.Product{
		ID:          uuid.New(),
		DisplayName: "Test Product",
		IsActive:    true,
		EntitlementsSpec: map[string]*int{
			"test_access": intPtr(30),
		},
	}
	_, err := suite.BunDB.NewInsert().Model(product).Exec(ctx)
	require.NoError(t, err)

	price := &models.Price{
		ID:               uuid.New(),
		ProductID:        product.ID,
		DisplayName:      "Test Price",
		Amount:           100,
		Currency:         "usd",
		BillingCycleDays: intPtr(30),
		IsActive:         true,
	}
	_, err = suite.BunDB.NewInsert().Model(price).Exec(ctx)
	require.NoError(t, err)

	userID := uuid.New().String()

	// Create multiple grants for the user
	for i := 0; i < 3; i++ {
		body := fmt.Sprintf(`{
			"price_id": "%s",
			"reason": "Test grant %d"
		}`, api.FormatPriceID(price.ID), i)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+userID+"/grants", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)
	}

	t.Run("lists grants for user with pagination", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/users/"+userID+"/grants?limit=10", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "list", resp["object"])
		data := resp["data"].([]interface{})
		assert.Len(t, data, 3)
		assert.Equal(t, float64(3), resp["total"])
	})

	t.Run("returns empty list for user with no grants", func(t *testing.T) {
		otherUserID := uuid.New().String()

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/users/"+otherUserID+"/grants", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "list", resp["object"])
		data := resp["data"].([]interface{})
		assert.Len(t, data, 0)
		assert.Equal(t, float64(0), resp["total"])
	})
}

// TestGetAdminGrant tests GET /v1/admin/grants/:id
func TestGetAdminGrant(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	// Create a product and price
	ctx := context.Background()
	product := &models.Product{
		ID:          uuid.New(),
		DisplayName: "Test Product",
		IsActive:    true,
		EntitlementsSpec: map[string]*int{
			"test_access": intPtr(30),
		},
	}
	_, err := suite.BunDB.NewInsert().Model(product).Exec(ctx)
	require.NoError(t, err)

	price := &models.Price{
		ID:               uuid.New(),
		ProductID:        product.ID,
		DisplayName:      "Test Price",
		Amount:           100,
		Currency:         "usd",
		BillingCycleDays: intPtr(30),
		IsActive:         true,
	}
	_, err = suite.BunDB.NewInsert().Model(price).Exec(ctx)
	require.NoError(t, err)

	userID := uuid.New().String()

	// Create a grant
	body := fmt.Sprintf(`{
		"price_id": "%s",
		"reason": "Test grant"
	}`, api.FormatPriceID(price.ID))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/admin/users/"+userID+"/grants", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	suite.Server.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var createResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &createResp)
	require.NoError(t, err)
	grantID := createResp["admin_grant_id"].(string)

	t.Run("retrieves grant by ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/grants/"+grantID, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "admin_grant", resp["object"])
		assert.Equal(t, grantID, resp["id"])
		assert.Equal(t, userID, resp["user_id"])
		assert.Equal(t, "Test grant", resp["reason"])
	})

	t.Run("returns 404 for non-existent grant", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/grants/ag_"+uuid.New().String(), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestAdminGrantValidation tests request validation
func TestAdminGrantValidation(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)
	userID := uuid.New().String()

	t.Run("requires price_id", func(t *testing.T) {
		body := `{"reason": "Test"}`

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+userID+"/grants", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("requires reason", func(t *testing.T) {
		body := `{"price_id": "price_` + uuid.New().String() + `"}`

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+userID+"/grants", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("rejects invalid price_id", func(t *testing.T) {
		body := `{"price_id": "invalid", "reason": "Test"}`

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+userID+"/grants", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("rejects non-existent price_id", func(t *testing.T) {
		body := fmt.Sprintf(`{"price_id": "price_%s", "reason": "Test"}`, uuid.New().String())

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/admin/users/"+userID+"/grants", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code) // price not found
	})
}
