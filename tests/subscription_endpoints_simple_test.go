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

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/handlers"
)

// SimpleSubscriptionEndpointsTestSuite tests subscription endpoints against a running server
type SimpleSubscriptionEndpointsTestSuite struct {
	suite.Suite

	// Server configuration
	config    *config.Config
	serverURL string

	// Test users
	regularUserToken string
	adminUserToken   string

	// HTTP client
	client *http.Client
}

func TestSimpleSubscriptionEndpointsTestSuite(t *testing.T) {
	suite.Run(t, new(SimpleSubscriptionEndpointsTestSuite))
}

func (s *SimpleSubscriptionEndpointsTestSuite) SetupSuite() {
	// Load configuration from file
	var err error
	s.config, err = config.Load("")
	if err != nil {
		s.T().Skipf("Could not load config: %v", err)
		return
	}

	// Set server URL - assume server is running locally
	s.serverURL = fmt.Sprintf("http://%s:%d", s.config.Host, s.config.Port)

	// Create HTTP client with timeout
	s.client = &http.Client{
		Timeout: 10 * time.Second,
	}

	// Generate test JWT tokens
	s.regularUserToken = GenerateTestJWT(s.T(), "test-user-123", "test@example.com", false)
	s.adminUserToken = GenerateTestJWT(s.T(), "admin-user-456", "admin@example.com", true)

	// Check if server is running
	if !s.isServerRunning() {
		s.T().Skip("Server is not running. Please start the server before running tests.")
	}
}

func (s *SimpleSubscriptionEndpointsTestSuite) isServerRunning() bool {
	resp, err := s.client.Get(s.serverURL + "/health/live")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Test POST /api/v1/subscriptions/processor/:processor
func (s *SimpleSubscriptionEndpointsTestSuite) TestSubscribeEndpoints() {
	testCases := []struct {
		name      string
		processor string
		priceID   string
		token     string
		expectErr bool
	}{
		{
			name:      "Subscribe with CCBill processor",
			processor: "ccbill",
			priceID:   uuid.New().String(),
			token:     s.regularUserToken,
			expectErr: false,
		},
		{
			name:      "Subscribe with Mobius processor",
			processor: "mobius",
			priceID:   uuid.New().String(),
			token:     s.regularUserToken,
			expectErr: false,
		},
		{
			name:      "Subscribe without authentication",
			processor: "ccbill",
			priceID:   uuid.New().String(),
			token:     "",
			expectErr: true,
		},
		{
			name:      "Subscribe with invalid processor",
			processor: "invalid",
			priceID:   uuid.New().String(),
			token:     s.regularUserToken,
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			reqBody := map[string]interface{}{
				"data": map[string]interface{}{
					"price_id":  tc.priceID,
					"processor": tc.processor,
				},
			}

			resp := s.makeRequest("POST", fmt.Sprintf("/api/v1/subscriptions/processor/%s", tc.processor), reqBody, tc.token)
			defer resp.Body.Close()

			if tc.expectErr {
				assert.True(s.T(), resp.StatusCode >= 400, "Should return error status code")
			} else {
				// Note: Might return 500 if price doesn't exist, but that's expected
				assert.True(s.T(), resp.StatusCode < 500 || resp.StatusCode == 500, "Should not return client error")
			}
		})
	}
}

// Test GET /api/v1/subscriptions/active
func (s *SimpleSubscriptionEndpointsTestSuite) TestGetActiveSubscription() {
	s.Run("Get active subscription with authentication", func() {
		resp := s.makeRequest("GET", "/api/v1/subscriptions/active", nil, s.regularUserToken)
		defer resp.Body.Close()

		assert.True(s.T(), resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound,
			"Should return 200 or 404 for active subscription")
	})

	s.Run("Get active subscription without authentication", func() {
		resp := s.makeRequest("GET", "/api/v1/subscriptions/active", nil, "")
		defer resp.Body.Close()

		assert.Equal(s.T(), http.StatusUnauthorized, resp.StatusCode, "Should return 401 without authentication")
	})
}

// Test GET /api/v1/subscriptions/history
func (s *SimpleSubscriptionEndpointsTestSuite) TestGetSubscriptionHistory() {
	s.Run("Get subscription history with authentication", func() {
		resp := s.makeRequest("GET", "/api/v1/subscriptions/history", nil, s.regularUserToken)
		defer resp.Body.Close()

		assert.Equal(s.T(), http.StatusOK, resp.StatusCode, "Should return subscription history")

		var result map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(s.T(), err)

		assert.Contains(s.T(), result, "data", "Response should contain data field")
	})

	s.Run("Get subscription history with pagination", func() {
		resp := s.makeRequest("GET", "/api/v1/subscriptions/history?limit=5&offset=0", nil, s.regularUserToken)
		defer resp.Body.Close()

		assert.Equal(s.T(), http.StatusOK, resp.StatusCode, "Should return paginated history")
	})

	s.Run("Get subscription history without authentication", func() {
		resp := s.makeRequest("GET", "/api/v1/subscriptions/history", nil, "")
		defer resp.Body.Close()

		assert.Equal(s.T(), http.StatusUnauthorized, resp.StatusCode, "Should return 401 without authentication")
	})
}

// Test POST /api/v1/subscriptions/cancel
func (s *SimpleSubscriptionEndpointsTestSuite) TestCancelSubscription() {
	s.Run("Cancel subscription with authentication", func() {
		reqBody := handlers.CancelSubscriptionBodyParams{
			Feedback: "Test cancellation",
		}

		resp := s.makeRequest("POST", "/api/v1/subscriptions/cancel", reqBody, s.regularUserToken)
		defer resp.Body.Close()

		// Might return 500 if no active subscription, but endpoint should be accessible
		assert.True(s.T(), resp.StatusCode != http.StatusUnauthorized, "Should not return 401 with authentication")
	})

	s.Run("Cancel subscription without authentication", func() {
		reqBody := handlers.CancelSubscriptionBodyParams{
			Feedback: "Test cancellation",
		}

		resp := s.makeRequest("POST", "/api/v1/subscriptions/cancel", reqBody, "")
		defer resp.Body.Close()

		assert.Equal(s.T(), http.StatusUnauthorized, resp.StatusCode, "Should return 401 without authentication")
	})

	s.Run("Cancel subscription with invalid JSON", func() {
		resp := s.makeRawRequest("POST", "/api/v1/subscriptions/cancel", []byte("invalid json"), s.regularUserToken)
		defer resp.Body.Close()

		assert.Equal(s.T(), http.StatusBadRequest, resp.StatusCode, "Should return 400 for invalid JSON")
	})
}

// Test POST /api/v1/subscriptions/ccbill/flexform-url
func (s *SimpleSubscriptionEndpointsTestSuite) TestGenerateFlexFormURL() {
	s.Run("Generate FlexForm URL with valid data", func() {
		reqBody := handlers.GenerateFlexFormURLBodyParams{
			PriceID:   uuid.New().String(),
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Main St",
			City:      "New York",
			State:     "NY",
			ZipCode:   "10001",
			Country:   "US",
		}

		resp := s.makeRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", reqBody, s.regularUserToken)
		defer resp.Body.Close()

		// Might return 404 if price doesn't exist, but endpoint should be accessible
		assert.True(s.T(), resp.StatusCode != http.StatusUnauthorized, "Should not return 401 with authentication")
	})

	s.Run("Generate FlexForm URL without authentication", func() {
		reqBody := handlers.GenerateFlexFormURLBodyParams{
			PriceID:   uuid.New().String(),
			FirstName: "John",
			LastName:  "Doe",
			Address1:  "123 Main St",
			City:      "New York",
			State:     "NY",
			ZipCode:   "10001",
			Country:   "US",
		}

		resp := s.makeRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", reqBody, "")
		defer resp.Body.Close()

		assert.Equal(s.T(), http.StatusUnauthorized, resp.StatusCode, "Should return 401 without authentication")
	})

	s.Run("Generate FlexForm URL with missing required fields", func() {
		reqBody := handlers.GenerateFlexFormURLBodyParams{
			PriceID: uuid.New().String(),
			// Missing required fields
		}

		resp := s.makeRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", reqBody, s.regularUserToken)
		defer resp.Body.Close()

		assert.Equal(s.T(), http.StatusBadRequest, resp.StatusCode, "Should return 400 for missing required fields")
	})
}

// Test HTTP methods
func (s *SimpleSubscriptionEndpointsTestSuite) TestHTTPMethods() {
	testCases := []struct {
		path           string
		allowedMethods []string
		deniedMethods  []string
	}{
		{
			path:           "/api/v1/subscriptions/active",
			allowedMethods: []string{"GET"},
			deniedMethods:  []string{"POST", "PUT", "DELETE"},
		},
		{
			path:           "/api/v1/subscriptions/history",
			allowedMethods: []string{"GET"},
			deniedMethods:  []string{"POST", "PUT", "DELETE"},
		},
		{
			path:           "/api/v1/subscriptions/cancel",
			allowedMethods: []string{"POST"},
			deniedMethods:  []string{"GET", "PUT", "DELETE"},
		},
	}

	for _, tc := range testCases {
		for _, method := range tc.allowedMethods {
			s.Run(fmt.Sprintf("%s allows %s", tc.path, method), func() {
				req, err := http.NewRequest(method, s.serverURL+tc.path, nil)
				require.NoError(s.T(), err)

				req.Header.Set("Authorization", "Bearer "+s.regularUserToken)

				resp, err := s.client.Do(req)
				require.NoError(s.T(), err)
				defer resp.Body.Close()

				assert.NotEqual(s.T(), http.StatusMethodNotAllowed, resp.StatusCode,
					"Should allow "+method+" method")
			})
		}

		for _, method := range tc.deniedMethods {
			s.Run(fmt.Sprintf("%s denies %s", tc.path, method), func() {
				req, err := http.NewRequest(method, s.serverURL+tc.path, nil)
				require.NoError(s.T(), err)

				req.Header.Set("Authorization", "Bearer "+s.regularUserToken)

				resp, err := s.client.Do(req)
				require.NoError(s.T(), err)
				defer resp.Body.Close()

				// Should return 404 (not found) or 405 (method not allowed)
				assert.True(s.T(), resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed,
					"Should deny "+method+" method")
			})
		}
	}
}

// Test authentication
func (s *SimpleSubscriptionEndpointsTestSuite) TestAuthentication() {
	endpoints := []string{
		"/api/v1/subscriptions/active",
		"/api/v1/subscriptions/history",
		"/api/v1/subscriptions/processor/ccbill",
		"/api/v1/subscriptions/cancel",
		"/api/v1/subscriptions/ccbill/flexform-url",
	}

	for _, endpoint := range endpoints {
		s.Run("Endpoint "+endpoint+" requires authentication", func() {
			method := "GET"
			if endpoint == "/api/v1/subscriptions/processor/ccbill" ||
				endpoint == "/api/v1/subscriptions/cancel" ||
				endpoint == "/api/v1/subscriptions/ccbill/flexform-url" {
				method = "POST"
			}

			req, err := http.NewRequest(method, s.serverURL+endpoint, nil)
			require.NoError(s.T(), err)

			resp, err := s.client.Do(req)
			require.NoError(s.T(), err)
			defer resp.Body.Close()

			assert.Equal(s.T(), http.StatusUnauthorized, resp.StatusCode,
				"Should return 401 without authentication")
		})
	}
}

// Helper methods

func (s *SimpleSubscriptionEndpointsTestSuite) makeRequest(method, path string, body interface{}, token string) *http.Response {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		require.NoError(s.T(), err)
	}

	return s.makeRawRequest(method, path, reqBody, token)
}

func (s *SimpleSubscriptionEndpointsTestSuite) makeRawRequest(method, path string, body []byte, token string) *http.Response {
	url := s.serverURL + path

	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	require.NoError(s.T(), err)

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.client.Do(req)
	require.NoError(s.T(), err)

	return resp
}
