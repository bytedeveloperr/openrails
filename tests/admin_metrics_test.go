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

func seedMetricsData(t *testing.T, suite *TestContainerSuite, priceID uuid.UUID) {
	t.Helper()
	userID := uuid.New().String()
	sub := suite.CreateTestSubscriptionWithOptions(SubscriptionOptions{
		UserID:         userID,
		PriceID:        priceID,
		Status:         models.StatusActive,
		Processor:      models.ProcessorMobius,
		ProcessorSubID: "metrics-" + uuid.NewString()[:8],
	})

	suite.CreateTestPaymentWithOptions(PaymentOptions{
		UserID:        userID,
		PriceID:       priceID,
		SubscriptionID: &sub.ID,
		Processor:     models.ProcessorMobius,
		Amount:        999,
		TransactionID: "txn-" + uuid.NewString()[:8],
	})
}

func TestAdminMetricsSummary(t *testing.T) {
	t.Parallel()
	suite, adminToken := setupAdminTestSuite(t)
	products := suite.SeedProducts()
	priceID := products[0].Prices[0].ID
	seedMetricsData(t, suite, priceID)

	query := "/v1/admin/metrics/summary"
	req, _ := http.NewRequest("GET", query, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	suite.Server.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "summary response")

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "mrr")
	assert.Contains(t, resp, "total_revenue")
	assert.Contains(t, resp, "active_subscriptions")
}

func TestAdminMetricsRevenue(t *testing.T) {
	t.Parallel()
	suite, adminToken := setupAdminTestSuite(t)
	products := suite.SeedProducts()
	seedMetricsData(t, suite, products[0].Prices[0].ID)
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -7).Format("2006-01-02")
	end := now.Format("2006-01-02")

	req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/admin/metrics/revenue?start=%s&end=%s&granularity=day", start, end), nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	suite.Server.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "buckets")
}

func TestAdminMetricsSubscriptions(t *testing.T) {
	t.Parallel()
	suite, adminToken := setupAdminTestSuite(t)
	products := suite.SeedProducts()
	seedMetricsData(t, suite, products[0].Prices[0].ID)

	req, _ := http.NewRequest("GET", "/v1/admin/metrics/subscriptions?granularity=day", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	suite.Server.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "buckets")
}

func TestAdminMetricsProcessors(t *testing.T) {
	t.Parallel()
	suite, adminToken := setupAdminTestSuite(t)
	products := suite.SeedProducts()
	seedMetricsData(t, suite, products[0].Prices[0].ID)

	req, _ := http.NewRequest("GET", "/v1/admin/metrics/processors", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	suite.Server.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "processors")
}

func TestAdminMetricsChurn(t *testing.T) {
	t.Parallel()
	suite, adminToken := setupAdminTestSuite(t)
	products := suite.SeedProducts()
	seedMetricsData(t, suite, products[0].Prices[0].ID)

	req, _ := http.NewRequest("GET", "/v1/admin/metrics/churn", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	suite.Server.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp, "monthly_churn")
}
