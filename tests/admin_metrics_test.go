//go:build integration

package tests

import (
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

// TestAdminDashboardMetrics tests the GET dashboard metrics endpoint
func TestAdminDashboardMetrics(t *testing.T) {
	suite := setupAdminTestSuite(t)

	// Seed some test data
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID

	// Create subscriptions with various statuses
	userID1 := uuid.New().String()
	userID2 := uuid.New().String()
	userID3 := uuid.New().String()

	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID1,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorNMI,
		ProcessorSubID: "metrics-active-1-" + uuid.New().String()[:8],
	})

	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID2,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorCCBill,
		ProcessorSubID: "metrics-active-2-" + uuid.New().String()[:8],
	})

	suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID3,
		PriceID:        priceID,
		Status:         models.StatusCancelled,
		Processor:      models.ProcessorNMI,
		ProcessorSubID: "metrics-cancelled-1-" + uuid.New().String()[:8],
	})

	t.Run("returns dashboard metrics", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/subscriptions/dashboard-metrics", nil)
		req.Header.Set("X-API-KEY", testAdminAPIKey)

		suite.Server.AdminHandler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify basic structure - actual response has these fields:
		// active_users_with_auto_renew, active_users_with_failing_rebill, active_users_without_auto_renew
		assert.Contains(t, response, "active_users_with_auto_renew", "Should contain active_users_with_auto_renew")
		assert.Contains(t, response, "active_users_without_auto_renew", "Should contain active_users_without_auto_renew")
	})
}

// TestAdminDailyMetrics tests the GET daily metrics endpoint
func TestAdminDailyMetrics(t *testing.T) {
	suite := setupAdminTestSuite(t)

	t.Run("returns daily metrics for date range", func(t *testing.T) {
		now := time.Now()
		startDate := now.AddDate(0, 0, -7).Format("2006-01-02")
		endDate := now.Format("2006-01-02")

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/subscriptions/daily-metrics?start=%s&end=%s", startDate, endDate), nil)
		req.Header.Set("X-API-KEY", testAdminAPIKey)

		suite.Server.AdminHandler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response []map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Response should be an array of daily metrics
		// It can be empty if no data for the period
	})

	t.Run("returns error for missing start date", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/subscriptions/daily-metrics?end=2025-01-01", nil)
		req.Header.Set("X-API-KEY", testAdminAPIKey)

		suite.Server.AdminHandler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns error for missing end date", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/subscriptions/daily-metrics?start=2025-01-01", nil)
		req.Header.Set("X-API-KEY", testAdminAPIKey)

		suite.Server.AdminHandler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})

	t.Run("returns error for invalid date format", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/subscriptions/daily-metrics?start=01-01-2025&end=01-31-2025", nil)
		req.Header.Set("X-API-KEY", testAdminAPIKey)

		suite.Server.AdminHandler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 Bad Request")
	})
}

// TestAdminProcessorMetrics tests the GET processor metrics endpoint
func TestAdminProcessorMetrics(t *testing.T) {
	suite := setupAdminTestSuite(t)

	t.Run("returns processor metrics", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/v1/subscriptions/processor-metrics", nil)
		req.Header.Set("X-API-KEY", testAdminAPIKey)

		suite.Server.AdminHandler().ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "Should return 200 OK, got: %s", w.Body.String())

		var response []map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Response is an array of processor-specific metrics
		// Verify it's parseable; specific values depend on test data
	})
}
