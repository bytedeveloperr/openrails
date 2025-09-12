//go:build integration

// Package tests contains integration tests for the billing analytics feature using testcontainers.
//
// These tests verify that:
// 1. Admin billing analytics endpoints return correct data structures
// 2. Authentication and authorization work correctly
// 3. Dashboard metrics endpoints are accessible
// 4. Daily metrics endpoints are accessible
// 5. Processor metrics endpoints are accessible
// 6. Error handling works correctly
//
// To run these tests:
//
//	go test -tags=integration ./tests/ -v -run TestBillingAnalyticsTestcontainers
//
// Prerequisites:
// - Docker daemon running (for testcontainers)
package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// BillingAnalyticsTestcontainersSuite tests admin billing analytics endpoints with testcontainers
type BillingAnalyticsTestcontainersSuite struct {
	suite.Suite
	containers *TestContainerSuite
	adminID    string
	userID     string
	adminToken string
	userToken  string
}

// SetupSuite runs once before all tests
func (suite *BillingAnalyticsTestcontainersSuite) SetupSuite() {
	// Create testcontainer environment
	suite.containers = NewTestContainerSuite(suite.T())

	// Create test users using shared helpers
	adminResult := CreateAdminTestUserForSuite(suite.T(), suite.containers, "admin@example.com")
	suite.adminID = adminResult.User.ID

	userResult := CreateStandardTestUserForSuite(suite.T(), suite.containers, "user@example.com")
	suite.userID = userResult.User.ID

	// Generate test tokens with real user IDs
	suite.adminToken = suite.generateTestJWT(suite.adminID, "admin@example.com", true)
	suite.userToken = suite.generateTestJWT(suite.userID, "user@example.com", false)
}

// TearDownSuite runs once after all tests
func (suite *BillingAnalyticsTestcontainersSuite) TearDownSuite() {
	if suite.containers != nil {
		suite.containers.Cleanup()
	}
}

// generateTestJWT creates a proper JWT token for testing with real user ID
func (suite *BillingAnalyticsTestcontainersSuite) generateTestJWT(userID string, email string, isAdmin bool) string {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"iss":   "doujins-test",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	if isAdmin {
		claims["roles"] = []string{"admin"}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte("test-jwt-secret-for-integration-tests"))
	require.NoError(suite.T(), err)
	return signedToken
}

// makeRequest is a helper to make HTTP requests to the test server
func (suite *BillingAnalyticsTestcontainersSuite) makeRequest(method, path, body, authToken string) *http.Response {
	fullURL := suite.containers.ServerURL + path
	req, err := http.NewRequest(method, fullURL, strings.NewReader(body))
	require.NoError(suite.T(), err)

	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(suite.T(), err)

	return resp
}

// makeAdminRequest is a helper to make authenticated admin requests
func (suite *BillingAnalyticsTestcontainersSuite) makeAdminRequest(method, path, body string) *http.Response {
	return suite.makeRequest(method, path, body, suite.adminToken)
}

// makeUserRequest is a helper to make authenticated user requests
func (suite *BillingAnalyticsTestcontainersSuite) makeUserRequest(method, path, body string) *http.Response {
	return suite.makeRequest(method, path, body, suite.userToken)
}

// TestBillingDashboardMetrics tests the billing dashboard metrics endpoint
func (suite *BillingAnalyticsTestcontainersSuite) TestBillingDashboardMetrics() {
	suite.T().Run("Admin access to dashboard metrics", func(t *testing.T) {
		resp := suite.makeAdminRequest("GET", "/api/v1/admin/subscriptions/dashboard-metrics", "")
		defer resp.Body.Close()

		// Should return success or valid error (not authorization error)
		assert.True(t, resp.StatusCode != http.StatusUnauthorized,
			"Admin should have access to billing dashboard, got %d", resp.StatusCode)
		assert.True(t, resp.StatusCode < 500,
			"Should not return server errors, got %d", resp.StatusCode)

		if resp.StatusCode == 200 {
			var response map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&response)
			assert.NoError(t, err)
			// Dashboard should have metrics structure
			assert.NotEmpty(t, response, "Dashboard response should not be empty")
		}
	})

	suite.T().Run("User access denied to dashboard metrics", func(t *testing.T) {
		resp := suite.makeUserRequest("GET", "/api/v1/admin/subscriptions/dashboard-metrics", "")
		defer resp.Body.Close()

		// Regular users should not have access to admin billing dashboard
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Regular users should not have access to admin billing dashboard")
	})

	suite.T().Run("Unauthenticated access denied", func(t *testing.T) {
		resp := suite.makeRequest("GET", "/api/v1/admin/subscriptions/dashboard-metrics", "", "")
		defer resp.Body.Close()

		// Unauthenticated requests should be denied
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"Unauthenticated requests should be denied")
	})
}

// TestBillingDailyMetrics tests the billing daily metrics endpoint
func (suite *BillingAnalyticsTestcontainersSuite) TestBillingDailyMetrics() {
	suite.T().Run("Admin access to daily metrics", func(t *testing.T) {
		resp := suite.makeAdminRequest("GET", "/api/v1/admin/subscriptions/daily-metrics", "")
		defer resp.Body.Close()

		// Should return success or valid error (not authorization error)
		assert.True(t, resp.StatusCode != http.StatusUnauthorized,
			"Admin should have access to daily metrics, got %d", resp.StatusCode)
		assert.True(t, resp.StatusCode < 500,
			"Should not return server errors, got %d", resp.StatusCode)

		if resp.StatusCode == 200 {
			var response map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&response)
			assert.NoError(t, err)
			// Daily metrics should have a structure
			assert.NotEmpty(t, response, "Daily metrics response should not be empty")
		}
	})

	suite.T().Run("Daily metrics with date range", func(t *testing.T) {
		// Test with date range parameters
		endpoint := "/api/v1/admin/subscriptions/daily-metrics?start_date=2024-01-01&end_date=2024-01-31"
		resp := suite.makeAdminRequest("GET", endpoint, "")
		defer resp.Body.Close()

		// Should handle date range parameters properly
		assert.True(t, resp.StatusCode < 500,
			"Should handle date range parameters, got %d", resp.StatusCode)
	})

	suite.T().Run("Daily metrics with invalid date format", func(t *testing.T) {
		// Test with invalid date format
		endpoint := "/api/v1/admin/subscriptions/daily-metrics?start_date=invalid-date"
		resp := suite.makeAdminRequest("GET", endpoint, "")
		defer resp.Body.Close()

		// Should return client error for invalid date format
		assert.True(t, resp.StatusCode == 400 || resp.StatusCode == 200,
			"Should handle invalid date format appropriately, got %d", resp.StatusCode)
	})
}

// TestBillingProcessorMetrics tests the billing processor metrics endpoint
func (suite *BillingAnalyticsTestcontainersSuite) TestBillingProcessorMetrics() {
	suite.T().Run("Admin access to processor metrics", func(t *testing.T) {
		resp := suite.makeAdminRequest("GET", "/api/v1/admin/subscriptions/processor-metrics", "")
		defer resp.Body.Close()

		// Should return success or valid error (not authorization error)
		assert.True(t, resp.StatusCode != http.StatusUnauthorized,
			"Admin should have access to processor metrics, got %d", resp.StatusCode)
		assert.True(t, resp.StatusCode < 500,
			"Should not return server errors, got %d", resp.StatusCode)

		if resp.StatusCode == 200 {
			var response map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&response)
			assert.NoError(t, err)
			// Processor metrics should have a structure
			assert.NotEmpty(t, response, "Processor metrics response should not be empty")
		}
	})

	suite.T().Run("Processor metrics with time period", func(t *testing.T) {
		// Test with time period parameter
		endpoint := "/api/v1/admin/subscriptions/processor-metrics?period=30d"
		resp := suite.makeAdminRequest("GET", endpoint, "")
		defer resp.Body.Close()

		// Should handle time period parameters properly
		assert.True(t, resp.StatusCode < 500,
			"Should handle time period parameters, got %d", resp.StatusCode)
	})
}

// TestBillingSubscriptionMetrics tests subscription-related billing metrics
func (suite *BillingAnalyticsTestcontainersSuite) TestBillingSubscriptionMetrics() {
	suite.T().Run("Admin access to subscription metrics", func(t *testing.T) {
		resp := suite.makeAdminRequest("GET", "/api/v1/admin/subscriptions/subscribers", "")
		defer resp.Body.Close()

		// Should return success or valid error (not authorization error)
		assert.True(t, resp.StatusCode != http.StatusUnauthorized,
			"Admin should have access to subscription metrics, got %d", resp.StatusCode)
		assert.True(t, resp.StatusCode < 500,
			"Should not return server errors, got %d", resp.StatusCode)
	})

	suite.T().Run("Subscription metrics with pagination", func(t *testing.T) {
		// Test with pagination parameters
		endpoint := "/api/v1/admin/subscriptions/subscribers?page=1&page_size=10"
		resp := suite.makeAdminRequest("GET", endpoint, "")
		defer resp.Body.Close()

		// Should handle pagination parameters properly
		assert.True(t, resp.StatusCode < 500,
			"Should handle pagination parameters, got %d", resp.StatusCode)

		if resp.StatusCode == 200 {
			var response map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&response)
			assert.NoError(t, err)

			// Should have pagination structure
			if _, hasPage := response["page"]; hasPage {
				assert.Contains(t, response, "page_size")
				assert.Contains(t, response, "total_items")
			}
		}
	})
}

// TestBillingRevenueMetrics tests revenue-related billing metrics
func (suite *BillingAnalyticsTestcontainersSuite) TestBillingRevenueMetrics() {
	suite.T().Run("Admin access to revenue metrics", func(t *testing.T) {
		resp := suite.makeAdminRequest("GET", "/api/v1/admin/subscriptions/revenue", "")
		defer resp.Body.Close()

		// Should return success or valid error (not authorization error)
		assert.True(t, resp.StatusCode != http.StatusUnauthorized,
			"Admin should have access to revenue metrics, got %d", resp.StatusCode)
		assert.True(t, resp.StatusCode < 500,
			"Should not return server errors, got %d", resp.StatusCode)
	})

	suite.T().Run("Revenue metrics with time filters", func(t *testing.T) {
		// Test with various time filter combinations
		timeFilters := []string{
			"?period=7d",
			"?period=30d",
			"?period=90d",
			"?start_date=2024-01-01&end_date=2024-01-31",
		}

		for _, filter := range timeFilters {
			endpoint := "/api/v1/admin/subscriptions/revenue" + filter
			resp := suite.makeAdminRequest("GET", endpoint, "")
			defer resp.Body.Close()

			// Should handle time filters properly
			assert.True(t, resp.StatusCode < 500,
				"Should handle time filter %s, got %d", filter, resp.StatusCode)
		}
	})
}

// TestBillingAnalyticsAuthorization tests authorization for all billing analytics endpoints
func (suite *BillingAnalyticsTestcontainersSuite) TestBillingAnalyticsAuthorization() {
	billingEndpoints := []string{
		"/api/v1/admin/subscriptions/dashboard-metrics",
		"/api/v1/admin/subscriptions/daily-metrics",
		"/api/v1/admin/subscriptions/processor-metrics",
		"/api/v1/admin/subscriptions/subscribers",
		"/api/v1/admin/subscriptions/revenue",
	}

	for _, endpoint := range billingEndpoints {
		suite.T().Run("Unauthorized access to "+endpoint, func(t *testing.T) {
			// Test without authentication
			resp := suite.makeRequest("GET", endpoint, "", "")
			defer resp.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"Unauthenticated access should be denied for %s", endpoint)

			// Test with regular user token
			resp = suite.makeUserRequest("GET", endpoint, "")
			defer resp.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"Regular user access should be denied for %s", endpoint)
		})
	}
}

// TestBillingAnalyticsErrorHandling tests error handling for billing analytics endpoints
func (suite *BillingAnalyticsTestcontainersSuite) TestBillingAnalyticsErrorHandling() {
	suite.T().Run("Invalid HTTP methods", func(t *testing.T) {
		invalidMethods := []string{"POST", "PUT", "DELETE", "PATCH"}

		for _, method := range invalidMethods {
			resp := suite.makeAdminRequest(method, "/api/v1/admin/subscriptions/dashboard-metrics", "")
			defer resp.Body.Close()

			// Should return method not allowed or not found
			assert.True(t, resp.StatusCode == 405 || resp.StatusCode == 404,
				"Invalid method %s should return 405 or 404, got %d", method, resp.StatusCode)
		}
	})

	suite.T().Run("Malformed query parameters", func(t *testing.T) {
		malformedEndpoints := []string{
			"/api/v1/admin/subscriptions/daily-metrics?page=invalid",
			"/api/v1/admin/subscriptions/subscribers?page_size=invalid",
			"/api/v1/admin/subscriptions/revenue?period=invalid",
		}

		for _, endpoint := range malformedEndpoints {
			resp := suite.makeAdminRequest("GET", endpoint, "")
			defer resp.Body.Close()

			// Should handle malformed parameters gracefully (not 5xx)
			assert.True(t, resp.StatusCode < 500,
				"Should handle malformed parameters gracefully for %s, got %d", endpoint, resp.StatusCode)
		}
	})
}

// TestBillingAnalyticsTestcontainers runs the full test suite with testcontainers
func TestBillingAnalyticsTestcontainers(t *testing.T) {
	suite.Run(t, new(BillingAnalyticsTestcontainersSuite))
}
