//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/internal/services"
)

// SubscriptionEndpointsTestSuite tests all subscription management endpoints
type SubscriptionEndpointsTestSuite struct {
	suite.Suite
	testSuite *TestContainerSuite

	// Test users with different roles
	regularUser TestUserWithToken
	adminUser   TestUserWithToken

	// Test data
	testPrice   *models.Price
	testProduct *models.Product
	testPriceID string
}

type TestUserWithToken struct {
	ID    string
	Email string
	Token string
}

func TestSubscriptionEndpointsTestSuite(t *testing.T) {
	suite.Run(t, new(SubscriptionEndpointsTestSuite))
}

func (s *SubscriptionEndpointsTestSuite) SetupSuite() {
	s.testSuite = NewTestContainerSuite(s.T())

	// Create test users
	s.regularUser = TestUserWithToken{
		ID:    "test-user-123",
		Email: "test@example.com",
		Token: GenerateTestJWT(s.T(), "test-user-123", "test@example.com", false),
	}

	s.adminUser = TestUserWithToken{
		ID:    "admin-user-456",
		Email: "admin@example.com",
		Token: GenerateTestJWT(s.T(), "admin-user-456", "admin@example.com", true),
	}

	// Create test product and price
	s.setupTestData()
}

func (s *SubscriptionEndpointsTestSuite) TearDownSuite() {
	if s.testSuite != nil {
		s.testSuite.Cleanup()
	}
}

func (s *SubscriptionEndpointsTestSuite) SetupTest() {
	// Reset database state before each test
	s.testSuite.ResetDatabase()
	s.setupTestData() // Recreate test data after reset
}

func (s *SubscriptionEndpointsTestSuite) setupTestData() {
	// Create test product
	s.testProduct = &models.Product{
		ID:          uuid.New(),
		Slug:        "test-premium-plan",
		DisplayName: "Test Premium Plan",
		Description: "Test premium subscription plan",
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err := s.testSuite.BunDB.NewInsert().Model(s.testProduct).Exec(s.testSuite.ctx)
	require.NoError(s.T(), err)

	// Create test price
	billingCycleDays := 30
	s.testPrice = &models.Price{
		ID:               uuid.New(),
		ProductID:        s.testProduct.ID,
		DisplayName:      "Monthly Premium",
		Amount:           29.99,
		Currency:         "USD",
		BillingCycleDays: &billingCycleDays,
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	_, err = s.testSuite.BunDB.NewInsert().Model(s.testPrice).Exec(s.testSuite.ctx)
	require.NoError(s.T(), err)

	s.testPriceID = s.testPrice.ID.String()
}

// Test POST /api/v1/subscriptions/processor/:processor
func (s *SubscriptionEndpointsTestSuite) TestSubscribeEndpoint() {
	s.Run("Subscribe with CCBill processor", func() {
		subscribeData := services.SubscribeData{
			PriceID:   s.testPriceID,
			Processor: "ccbill",
		}

		reqBody := handlers.SubscribeBodyParams{
			Data: subscribeData,
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/processor/ccbill", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should successfully create CCBill subscription")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		assert.Contains(s.T(), result, "data", "Response should contain data field")
	})

	s.Run("Subscribe with Mobius processor", func() {
		subscribeData := services.SubscribeData{
			PriceID:   s.testPriceID,
			Processor: "mobius",
		}

		reqBody := handlers.SubscribeBodyParams{
			Data: subscribeData,
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/processor/mobius", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should successfully create Mobius subscription")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		assert.Contains(s.T(), result, "data", "Response should contain data field")
	})

	s.Run("Subscribe without authentication", func() {
		subscribeData := services.SubscribeData{
			PriceID:   s.testPriceID,
			Processor: "ccbill",
		}

		reqBody := handlers.SubscribeBodyParams{
			Data: subscribeData,
		}

		response := s.makeRequest("POST", "/api/v1/subscriptions/processor/ccbill", reqBody, "")
		assert.Equal(s.T(), http.StatusUnauthorized, response.StatusCode, "Should return 401 without authentication")
	})

	s.Run("Subscribe with invalid processor", func() {
		subscribeData := services.SubscribeData{
			PriceID:   s.testPriceID,
			Processor: "invalid-processor",
		}

		reqBody := handlers.SubscribeBodyParams{
			Data: subscribeData,
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/processor/invalid-processor", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusBadRequest, response.StatusCode, "Should return 400 for invalid processor")
	})

	s.Run("Subscribe with invalid price ID", func() {
		subscribeData := services.SubscribeData{
			PriceID:   "invalid-uuid",
			Processor: "ccbill",
		}

		reqBody := handlers.SubscribeBodyParams{
			Data: subscribeData,
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/processor/ccbill", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusBadRequest, response.StatusCode, "Should return 400 for invalid price ID")
	})

	s.Run("Subscribe with missing price ID", func() {
		subscribeData := services.SubscribeData{
			Processor: "ccbill",
		}

		reqBody := handlers.SubscribeBodyParams{
			Data: subscribeData,
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/processor/ccbill", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusBadRequest, response.StatusCode, "Should return 400 for missing price ID")
	})
}

// Test GET /api/v1/subscriptions/active
func (s *SubscriptionEndpointsTestSuite) TestGetActiveSubscription() {
	s.Run("Get active subscription when user has one", func() {
		// Create an active subscription for the user
		subscription := s.createTestSubscription(s.regularUser.ID, models.StatusActive)

		response := s.makeAuthenticatedRequest("GET", "/api/v1/subscriptions/active", nil, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should return active subscription")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		data, ok := result["data"].(map[string]interface{})
		require.True(s.T(), ok, "Response should contain data object")
		assert.Equal(s.T(), subscription.ID.String(), data["id"], "Should return correct subscription ID")
		assert.Equal(s.T(), string(models.StatusActive), data["status"], "Should return active status")
	})

	s.Run("Get active subscription when user has none", func() {
		response := s.makeAuthenticatedRequest("GET", "/api/v1/subscriptions/active", nil, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should return success even with no subscription")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		assert.Contains(s.T(), result, "message", "Should contain message about no active subscription")
	})

	s.Run("Get active subscription without authentication", func() {
		response := s.makeRequest("GET", "/api/v1/subscriptions/active", nil, "")
		assert.Equal(s.T(), http.StatusUnauthorized, response.StatusCode, "Should return 401 without authentication")
	})

	s.Run("Get active subscription with cancelled subscription", func() {
		// Create a cancelled subscription for the user
		s.createTestSubscription(s.regularUser.ID, models.StatusCancelled)

		response := s.makeAuthenticatedRequest("GET", "/api/v1/subscriptions/active", nil, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should return success")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		assert.Contains(s.T(), result, "message", "Should contain message about no active subscription")
	})
}

// Test GET /api/v1/subscriptions/history
func (s *SubscriptionEndpointsTestSuite) TestGetSubscriptionHistory() {
	s.Run("Get subscription history with multiple subscriptions", func() {
		// Create multiple subscriptions for the user
		sub1 := s.createTestSubscription(s.regularUser.ID, models.StatusActive)
		sub2 := s.createTestSubscription(s.regularUser.ID, models.StatusCancelled)

		response := s.makeAuthenticatedRequest("GET", "/api/v1/subscriptions/history", nil, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should return subscription history")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		data, ok := result["data"].([]interface{})
		require.True(s.T(), ok, "Response should contain data array")
		assert.GreaterOrEqual(s.T(), len(data), 2, "Should return at least 2 subscriptions")

		// Verify subscription IDs are present
		foundSub1, foundSub2 := false, false
		for _, item := range data {
			sub := item.(map[string]interface{})
			if sub["id"] == sub1.ID.String() {
				foundSub1 = true
			}
			if sub["id"] == sub2.ID.String() {
				foundSub2 = true
			}
		}
		assert.True(s.T(), foundSub1, "Should find first subscription in history")
		assert.True(s.T(), foundSub2, "Should find second subscription in history")
	})

	s.Run("Get subscription history with pagination", func() {
		// Create multiple subscriptions
		for i := 0; i < 5; i++ {
			s.createTestSubscription(s.regularUser.ID, models.StatusActive)
		}

		response := s.makeAuthenticatedRequest("GET", "/api/v1/subscriptions/history?limit=2&offset=0", nil, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should return paginated history")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		data, ok := result["data"].([]interface{})
		require.True(s.T(), ok, "Response should contain data array")
		assert.LessOrEqual(s.T(), len(data), 2, "Should respect limit parameter")
	})

	s.Run("Get subscription history with status filter", func() {
		// Create subscriptions with different statuses
		s.createTestSubscription(s.regularUser.ID, models.StatusActive)
		s.createTestSubscription(s.regularUser.ID, models.StatusCancelled)

		response := s.makeAuthenticatedRequest("GET", "/api/v1/subscriptions/history?status=active", nil, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should return filtered history")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		data, ok := result["data"].([]interface{})
		require.True(s.T(), ok, "Response should contain data array")

		// Verify all returned subscriptions have active status
		for _, item := range data {
			sub := item.(map[string]interface{})
			assert.Equal(s.T(), "active", sub["status"], "All subscriptions should be active")
		}
	})

	s.Run("Get subscription history without authentication", func() {
		response := s.makeRequest("GET", "/api/v1/subscriptions/history", nil, "")
		assert.Equal(s.T(), http.StatusUnauthorized, response.StatusCode, "Should return 401 without authentication")
	})

	s.Run("Get subscription history with no subscriptions", func() {
		response := s.makeAuthenticatedRequest("GET", "/api/v1/subscriptions/history", nil, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should return success even with no subscriptions")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		data, ok := result["data"].([]interface{})
		require.True(s.T(), ok, "Response should contain data array")
		assert.Equal(s.T(), 0, len(data), "Should return empty array")
	})
}

// Test POST /api/v1/subscriptions/cancel
func (s *SubscriptionEndpointsTestSuite) TestCancelSubscription() {
	s.Run("Cancel active subscription", func() {
		// Create an active subscription for the user
		s.createTestSubscription(s.regularUser.ID, models.StatusActive)

		reqBody := handlers.CancelSubscriptionBodyParams{
			Feedback: "Not satisfied with the service",
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/cancel", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should successfully cancel subscription")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		assert.Contains(s.T(), result, "message", "Response should contain success message")
		message, ok := result["message"].(string)
		require.True(s.T(), ok)
		assert.Contains(s.T(), message, "cancelled", "Message should indicate cancellation")
	})

	s.Run("Cancel subscription with empty feedback", func() {
		// Create an active subscription for the user
		s.createTestSubscription(s.regularUser.ID, models.StatusActive)

		reqBody := handlers.CancelSubscriptionBodyParams{
			Feedback: "",
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/cancel", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should successfully cancel subscription even with empty feedback")
	})

	s.Run("Cancel subscription when user has no active subscription", func() {
		reqBody := handlers.CancelSubscriptionBodyParams{
			Feedback: "Want to cancel",
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/cancel", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusInternalServerError, response.StatusCode, "Should return error when no active subscription")
	})

	s.Run("Cancel subscription without authentication", func() {
		reqBody := handlers.CancelSubscriptionBodyParams{
			Feedback: "Want to cancel",
		}

		response := s.makeRequest("POST", "/api/v1/subscriptions/cancel", reqBody, "")
		assert.Equal(s.T(), http.StatusUnauthorized, response.StatusCode, "Should return 401 without authentication")
	})

	s.Run("Cancel subscription with invalid request body", func() {
		// Create an active subscription for the user
		s.createTestSubscription(s.regularUser.ID, models.StatusActive)

		// Send invalid JSON
		response := s.makeAuthenticatedRequestRaw("POST", "/api/v1/subscriptions/cancel", []byte("invalid json"), s.regularUser.Token)
		assert.Equal(s.T(), http.StatusBadRequest, response.StatusCode, "Should return 400 for invalid JSON")
	})

	s.Run("Cancel subscription with feedback too long", func() {
		// Create an active subscription for the user
		s.createTestSubscription(s.regularUser.ID, models.StatusActive)

		// Create feedback longer than 500 characters
		longFeedback := string(make([]byte, 501))
		for i := range longFeedback {
			longFeedback = longFeedback[:i] + "a" + longFeedback[i+1:]
		}

		reqBody := handlers.CancelSubscriptionBodyParams{
			Feedback: longFeedback,
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/cancel", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusBadRequest, response.StatusCode, "Should return 400 for feedback too long")
	})
}

// Test POST /api/v1/subscriptions/ccbill/flexform-url
func (s *SubscriptionEndpointsTestSuite) TestGenerateFlexFormURL() {
	s.Run("Generate FlexForm URL with valid data", func() {
		reqBody := handlers.GenerateFlexFormURLBodyParams{
			PriceID:   s.testPriceID,
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Main St",
			City:      "New York",
			State:     "NY",
			ZipCode:   "10001",
			Country:   "US",
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusOK, response.StatusCode, "Should successfully generate FlexForm URL")

		var result map[string]interface{}
		err := json.NewDecoder(response.Body).Decode(&result)
		require.NoError(s.T(), err)

		data, ok := result["data"].(map[string]interface{})
		require.True(s.T(), ok, "Response should contain data object")
		assert.Contains(s.T(), data, "iframe_url", "Response should contain iframe_url")
		assert.Contains(s.T(), data, "width", "Response should contain width")
		assert.Contains(s.T(), data, "height", "Response should contain height")
	})

	s.Run("Generate FlexForm URL without authentication", func() {
		reqBody := handlers.GenerateFlexFormURLBodyParams{
			PriceID:   s.testPriceID,
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Main St",
			City:      "New York",
			State:     "NY",
			ZipCode:   "10001",
			Country:   "US",
		}

		response := s.makeRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", reqBody, "")
		assert.Equal(s.T(), http.StatusUnauthorized, response.StatusCode, "Should return 401 without authentication")
	})

	s.Run("Generate FlexForm URL with invalid price ID", func() {
		reqBody := handlers.GenerateFlexFormURLBodyParams{
			PriceID:   "invalid-uuid",
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Main St",
			City:      "New York",
			State:     "NY",
			ZipCode:   "10001",
			Country:   "US",
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusBadRequest, response.StatusCode, "Should return 400 for invalid price ID")
	})

	s.Run("Generate FlexForm URL with missing required fields", func() {
		reqBody := handlers.GenerateFlexFormURLBodyParams{
			PriceID: s.testPriceID,
			// Missing required fields
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusBadRequest, response.StatusCode, "Should return 400 for missing required fields")
	})

	s.Run("Generate FlexForm URL with invalid country code", func() {
		reqBody := handlers.GenerateFlexFormURLBodyParams{
			PriceID:   s.testPriceID,
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Main St",
			City:      "New York",
			State:     "NY",
			ZipCode:   "10001",
			Country:   "USA", // Should be 2-letter code
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusBadRequest, response.StatusCode, "Should return 400 for invalid country code")
	})

	s.Run("Generate FlexForm URL with non-existent price", func() {
		nonExistentPriceID := uuid.New().String()

		reqBody := handlers.GenerateFlexFormURLBodyParams{
			PriceID:   nonExistentPriceID,
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Main St",
			City:      "New York",
			State:     "NY",
			ZipCode:   "10001",
			Country:   "US",
		}

		response := s.makeAuthenticatedRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", reqBody, s.regularUser.Token)
		assert.Equal(s.T(), http.StatusNotFound, response.StatusCode, "Should return 404 for non-existent price")
	})
}

// Helper methods

func (s *SubscriptionEndpointsTestSuite) createTestSubscription(userID string, status models.SubscriptionStatus) *models.Subscription {
	subscription := &models.Subscription{
		ID:                      uuid.New(),
		UserID:                  userID,
		PriceID:                 s.testPrice.ID,
		Status:                  status,
		StartedAt:               time.Now(),
		Processor:               models.ProcessorCCBill,
		ProcessorSubscriptionID: fmt.Sprintf("test-sub-%s", uuid.New().String()[:8]),
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}

	if status == models.StatusActive {
		now := time.Now()
		endTime := now.Add(30 * 24 * time.Hour)
		subscription.CurrentPeriodStartsAt = &now
		subscription.CurrentPeriodEndsAt = &endTime
	}

	_, err := s.testSuite.BunDB.NewInsert().Model(subscription).Exec(s.testSuite.ctx)
	require.NoError(s.T(), err)

	return subscription
}

func (s *SubscriptionEndpointsTestSuite) makeRequest(method, path string, body interface{}, authToken string) *http.Response {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		require.NoError(s.T(), err)
	}

	return s.makeAuthenticatedRequestRaw(method, path, reqBody, authToken)
}

func (s *SubscriptionEndpointsTestSuite) makeAuthenticatedRequest(method, path string, body interface{}, authToken string) *http.Response {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		require.NoError(s.T(), err)
	}

	return s.makeAuthenticatedRequestRaw(method, path, reqBody, authToken)
}

func (s *SubscriptionEndpointsTestSuite) makeAuthenticatedRequestRaw(method, path string, body []byte, authToken string) *http.Response {
	url := s.testSuite.ServerURL + path

	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	require.NoError(s.T(), err)

	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(s.T(), err)

	return resp
}
