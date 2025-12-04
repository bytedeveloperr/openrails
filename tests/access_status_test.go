//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/internal/services"
)

// TestAccessStatusRequiresAuth tests that access status endpoint requires authentication
func TestAccessStatusRequiresAuth(t *testing.T) {
	suite := setupTestSuite(t)

	t.Run("returns 401 without auth token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/access", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("returns 401 with invalid token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/access", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})
}

// TestAccessStatusNoSubscription tests access status for user without subscription
func TestAccessStatusNoSubscription(t *testing.T) {
	suite, token, _ := setupTestSuiteWithAuth(t)

	t.Run("returns is_premium=false for user without subscription", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/access", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response handlers.AccessStatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.False(t, response.IsPremium, "IsPremium should be false")
		assert.Empty(t, response.Access, "Access should be empty")
	})
}

// TestAccessStatusWithActiveSubscription tests access status for user with active subscription
func TestAccessStatusWithActiveSubscription(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products and create an active subscription
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID
	processorSubID := "test-sub-access-" + t.Name()

	sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: processorSubID,
	})

	t.Run("returns is_premium=true with active subscription", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/access", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response handlers.AccessStatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response.IsPremium, "IsPremium should be true")
		require.Len(t, response.Access, 1, "Should have one access grant")

		grant := response.Access[0]
		assert.Equal(t, "subscription", grant.Kind, "Grant kind should be subscription")
		assert.Equal(t, "premium", grant.Entitlement, "Entitlement should be premium")
		assert.Equal(t, "ccbill", grant.Processor, "Processor should be ccbill")
		require.NotNil(t, grant.SubscriptionID, "SubscriptionID should be set")
		assert.Equal(t, sub.ID, *grant.SubscriptionID, "SubscriptionID should match")
		require.NotNil(t, grant.ProcessorSubscriptionID, "ProcessorSubscriptionID should be set")
		assert.Equal(t, processorSubID, *grant.ProcessorSubscriptionID, "ProcessorSubscriptionID should match")
	})
}

// TestAccessStatusWithCancelledSubscription tests access status for user with cancelled subscription
func TestAccessStatusWithCancelledSubscription(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products and create a cancelled subscription
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusCancelled,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: "test-cancelled-sub-" + t.Name(),
	})

	t.Run("returns is_premium=false with cancelled subscription", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/access", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response handlers.AccessStatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.False(t, response.IsPremium, "IsPremium should be false for cancelled subscription")
		assert.Empty(t, response.Access, "Access should be empty for cancelled subscription")
	})
}

// TestAccessStatusWithEntitlement tests access status for user with active entitlement
func TestAccessStatusWithEntitlement(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Create an active entitlement (not from subscription)
	now := time.Now()
	endAt := now.Add(30 * 24 * time.Hour)
	sourceType := models.EntitlementSourceAdmin
	entitlement := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: "premium",
		SourceType:  sourceType,
		StartAt:     now,
		EndAt:       &endAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := suite.BunDB.NewInsert().Model(entitlement).Exec(suite.ctx)
	require.NoError(t, err, "Failed to create entitlement")

	t.Run("returns is_premium=true with active entitlement", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/access", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response handlers.AccessStatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response.IsPremium, "IsPremium should be true with entitlement")
		require.Len(t, response.Access, 1, "Should have one access grant")

		grant := response.Access[0]
		assert.Equal(t, "entitlement", grant.Kind, "Grant kind should be entitlement")
		assert.Equal(t, "premium", grant.Entitlement, "Entitlement type should be premium")
		require.NotNil(t, grant.SourceType, "SourceType should be set")
		assert.Equal(t, models.EntitlementSourceAdmin, *grant.SourceType, "SourceType should be admin")
	})
}

// TestAccessStatusWithExpiredEntitlement tests access status for user with expired entitlement
func TestAccessStatusWithExpiredEntitlement(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Create an expired entitlement
	now := time.Now()
	startAt := now.Add(-60 * 24 * time.Hour)
	endAt := now.Add(-30 * 24 * time.Hour) // Ended 30 days ago
	sourceType := models.EntitlementSourceAdmin
	entitlement := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: "premium",
		SourceType:  sourceType,
		StartAt:     startAt,
		EndAt:       &endAt,
		CreatedAt:   startAt,
		UpdatedAt:   now,
	}

	_, err := suite.BunDB.NewInsert().Model(entitlement).Exec(suite.ctx)
	require.NoError(t, err, "Failed to create entitlement")

	t.Run("returns is_premium=false with expired entitlement", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/access", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response handlers.AccessStatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.False(t, response.IsPremium, "IsPremium should be false for expired entitlement")
		assert.Empty(t, response.Access, "Access should be empty for expired entitlement")
	})
}

// TestAccessStatusWithBothSubscriptionAndEntitlement tests access status with both sources
func TestAccessStatusWithBothSubscriptionAndEntitlement(t *testing.T) {
	suite, token, userID := setupTestSuiteWithAuth(t)

	// Seed products and create an active subscription
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorNMI,
		ProcessorSubID: "test-sub-both-" + t.Name(),
	})

	// Also create an entitlement from a different source (not subscription)
	now := time.Now()
	endAt := now.Add(30 * 24 * time.Hour)
	sourceType := models.EntitlementSourceAdmin
	entitlement := &models.Entitlement{
		ID:          uuid.New(),
		UserID:      userID,
		Entitlement: "premium",
		SourceType:  sourceType,
		StartAt:     now,
		EndAt:       &endAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := suite.BunDB.NewInsert().Model(entitlement).Exec(suite.ctx)
	require.NoError(t, err, "Failed to create entitlement")

	t.Run("returns both subscription and entitlement grants", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/access", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response handlers.AccessStatusResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response.IsPremium, "IsPremium should be true")
		require.Len(t, response.Access, 2, "Should have two access grants")

		// Find subscription grant and entitlement grant
		var subGrant, entGrant *services.UserAccessGrant
		for _, g := range response.Access {
			if g.Kind == "subscription" {
				subGrant = g
			} else if g.Kind == "entitlement" {
				entGrant = g
			}
		}

		require.NotNil(t, subGrant, "Should have subscription grant")
		require.NotNil(t, entGrant, "Should have entitlement grant")

		// Verify subscription grant
		require.NotNil(t, subGrant.SubscriptionID)
		assert.Equal(t, sub.ID, *subGrant.SubscriptionID)
		assert.Equal(t, "nmi", subGrant.Processor)

		// Verify entitlement grant
		require.NotNil(t, entGrant.SourceType)
		assert.Equal(t, models.EntitlementSourceAdmin, *entGrant.SourceType)
	})
}
