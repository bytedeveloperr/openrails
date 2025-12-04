//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/internal/db/models"
)

// setupAdminTestSuite sets up the test suite for admin tests.
// Admin endpoints require JWT with "admin" role (via AuthRequired + AdminRequired middleware).
func setupAdminTestSuite(t *testing.T) (*TestContainerSuite, string) {
	suite, token, _ := setupTestSuiteWithAdminAuth(t)
	return suite, token
}

// TestAdminEndpointsRequireAuth tests that admin endpoints require JWT with admin role
func TestAdminEndpointsRequireAuth(t *testing.T) {
	suite, _ := setupAdminTestSuite(t)

	t.Run("PUT extend subscription returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/v1/admin/subscriptions/"+uuid.New().String()+"/extend", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("GET dashboard metrics returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/subscriptions/dashboard-metrics", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("GET user entitlements returns 401 without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/users/"+uuid.New().String()+"/entitlements", nil)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "Should return 401 Unauthorized")
	})

	t.Run("returns 403 with non-admin JWT", func(t *testing.T) {
		// Create a regular user token (no admin role)
		userID := uuid.New().String()
		userToken := CreateUserToken(t, userID)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/admin/subscriptions/dashboard-metrics", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code, "Should return 403 Forbidden for non-admin user")
	})
}

// TestAdminExtendSubscription tests the PUT extend subscription endpoint
func TestAdminExtendSubscription(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	// Create test products and subscription
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID
	userID := uuid.New().String()

	t.Run("extends subscription successfully", func(t *testing.T) {
		// Create a fresh subscription for this test
		now := time.Now()
		periodEnd := now.Add(30 * 24 * time.Hour)
		sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:         userID,
			PriceID:        priceID,
			Status:         models.StatusActive,
			Processor:      models.ProcessorNMI,
			ProcessorSubID: "admin-extend-sub-" + uuid.New().String()[:8],
			PeriodStart:    now,
			PeriodEnd:      periodEnd,
		})

		// Extend by 30 days (30 * 24 hours in nanoseconds)
		extendDuration := 30 * 24 * time.Hour
		body, _ := json.Marshal(map[string]interface{}{
			"SubscriptionID": sub.ID.String(),
			"Duration":       int64(extendDuration),
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/admin/subscriptions/%s/extend", sub.ID.String()), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		// Verify the subscription was extended
		updatedSub := suite.GetSubscription(sub.ID)
		require.NotNil(t, updatedSub.CurrentPeriodEndsAt, "Period end should be set")

		// New end should be approximately 60 days from now (original 30 + extension 30)
		expectedEnd := periodEnd.Add(extendDuration)
		diff := updatedSub.CurrentPeriodEndsAt.Sub(expectedEnd)
		assert.True(t, diff < time.Minute && diff > -time.Minute,
			"Period should be extended by 30 days, got diff: %v", diff)
	})

	t.Run("fails to extend cancelled subscription", func(t *testing.T) {
		cancelledSub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
			UserID:         userID,
			PriceID:        priceID,
			Status:         models.StatusCancelled,
			Processor:      models.ProcessorNMI,
			ProcessorSubID: "admin-extend-cancelled-" + uuid.New().String()[:8],
		})

		body, _ := json.Marshal(map[string]interface{}{
			"SubscriptionID": cancelledSub.ID.String(),
			"Duration":       int64(30 * 24 * time.Hour),
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/admin/subscriptions/%s/extend", cancelledSub.ID.String()), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code, "Should fail for cancelled subscription")
	})
}

// TestAdminGetUserEntitlements tests the GET user entitlements endpoint
func TestAdminGetUserEntitlements(t *testing.T) {
	suite, adminToken := setupAdminTestSuite(t)

	t.Run("returns empty list for user with no entitlements", func(t *testing.T) {
		userID := uuid.New().String()

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/admin/users/%s/entitlements", userID), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response []interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Empty(t, response, "Should return empty array for user with no entitlements")
	})

	t.Run("returns entitlements for user", func(t *testing.T) {
		userID := uuid.New().String()

		// Create entitlements
		ent := suite.CreateTestEntitlement(userID, "premium", nil, models.EntitlementSourceAdmin)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/admin/users/%s/entitlements", userID), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		suite.Server.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response []map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		require.Len(t, response, 1, "Should return one entitlement")
		assert.Equal(t, ent.ID.String(), response[0]["id"], "Entitlement ID should match")
		assert.Equal(t, "premium", response[0]["entitlement"], "Entitlement type should be premium")
	})
}

// TestAdminHealth tests the health endpoint (public, no auth required)
func TestAdminHealth(t *testing.T) {
	suite, _ := setupAdminTestSuite(t)

	t.Run("health endpoint returns ok without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)
		// No auth header - health endpoint should be public

		suite.Server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Should return 200 OK")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "ok", response["status"], "Status should be ok")
		assert.Equal(t, "billing", response["service"], "Service should be billing")
	})
}
