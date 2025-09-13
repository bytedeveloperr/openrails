package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/handlers"
	"github.com/doujins-org/doujins-billing/internal/services"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// Helper function to create a real server instance for testing
func createTestServer(t *testing.T) *Server {
	// Try to load config, but if it fails, create minimal test config
	cfg, err := config.Load("")
	require.NoError(t, err)

	// if err != nil {
	// 	// Create minimal config for testing if .env/config files don't exist
	// 	cfg = &config.Config{
	// 		Env:  "development", // Use development to skip strict validation
	// 		Port: 8080,
	// 		JWT: &config.JWTConfig{
	// 			Secret:   "test-jwt-secret-for-testing",
	// 			Issuer:   "billing-test",
	// 			Audience: "billing-client",
	// 		},
	// 		DB: &config.DBConfig{
	// 			URL:     "postgres://test:test@localhost:5432/test",
	// 			Dialect: "postgres",
	// 		},
	// 		Mobius: &config.MobiusConfig{
	// 			SecurityKey:     "test-security-key",
	// 			TokenizationKey: "test-tokenization-key",
	// 			WebhookSecret:   "test-webhook-secret",
	// 			TestMode:        true,
	// 		},
	// 		CCBill: &config.CCBillConfig{
	// 			Salt:         "test-salt",
	// 			ClientAccNum: "123456",
	// 			TestMode:     true,
	// 		},
	// 	}
	// }

	server, err := New(cfg)
	require.NoError(t, err)

	return server
}

// Helper function to create JWT token for testing
func createTestToken(server *Server, userID, email string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"iss":   server.cfg.JWT.Issuer,
		"aud":   server.cfg.JWT.Audience,
	})

	tokenString, _ := token.SignedString([]byte(server.cfg.JWT.Secret))
	return tokenString
}

// TestGetProductsEndpoint tests the public products endpoint
func TestGetProductsEndpoint(t *testing.T) {
	server := createTestServer(t)
	defer server.Close(context.Background())

	t.Run("GetProducts_Request", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/subscriptions/public/products", nil)

		server.Handler().ServeHTTP(w, req)

		// Should not return 404 (route exists)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
		// Should return some response (OK or error due to missing DB)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)
	})
}

// TestGetSubscribePageDataEndpoint tests the subscribe page data endpoint
func TestGetSubscribePageDataEndpoint(t *testing.T) {
	server := createTestServer(t)
	defer server.Close(context.Background())

	t.Run("GetSubscribePageData_Request", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/subscriptions/public/subscribe-page-data", nil)

		server.Handler().ServeHTTP(w, req)

		// Should not return 404 (route exists)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
		// Should return some response (OK or error due to missing DB)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, w.Code)
	})
}

// TestSubscribeEndpoint tests the subscription endpoint with authentication
func TestSubscribeEndpoint(t *testing.T) {
	server := createTestServer(t)
	defer server.Close(context.Background())

	t.Run("Subscribe_RequiresAuth", func(t *testing.T) {
		subscribeData := handlers.SubscribeRequest{
			SubscribeBodyParams: handlers.SubscribeBodyParams{
				Data: services.SubscribeData{
					Processor: "mobius",
					PriceID:   uuid.New().String(),
				},
			},
		}

		body, _ := json.Marshal(subscribeData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/processor/mobius", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Subscribe_WithAuth", func(t *testing.T) {
		userID := uuid.New().String()
		email := "test@example.com"
		token := createTestToken(server, userID, email)

		subscribeData := handlers.SubscribeRequest{
			SubscribeBodyParams: handlers.SubscribeBodyParams{
				Data: services.SubscribeData{
					Processor: "mobius",
					PriceID:   uuid.New().String(),
				},
			},
		}

		body, _ := json.Marshal(subscribeData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/processor/mobius", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		server.Handler().ServeHTTP(w, req)

		// Should not be unauthorized with valid token
		assert.NotEqual(t, http.StatusUnauthorized, w.Code)
		// Will likely get internal server error due to missing database/services
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest}, w.Code)
	})
}

// TestCancelSubscriptionEndpoint tests the cancel subscription endpoint
func TestCancelSubscriptionEndpoint(t *testing.T) {
	server := createTestServer(t)
	defer server.Close(context.Background())

	t.Run("CancelSubscription_RequiresAuth", func(t *testing.T) {
		cancelData := handlers.CancelSubscriptionRequest{
			CancelSubscriptionBodyParams: handlers.CancelSubscriptionBodyParams{
				Feedback: "Too expensive",
			},
		}

		body, _ := json.Marshal(cancelData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/cancel", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("CancelSubscription_WithAuth", func(t *testing.T) {
		userID := uuid.New().String()
		email := "test@example.com"
		token := createTestToken(server, userID, email)

		cancelData := handlers.CancelSubscriptionRequest{
			CancelSubscriptionBodyParams: handlers.CancelSubscriptionBodyParams{
				Feedback: "Too expensive",
			},
		}

		body, _ := json.Marshal(cancelData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/cancel", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		server.Handler().ServeHTTP(w, req)

		// Should not be unauthorized with valid token
		assert.NotEqual(t, http.StatusUnauthorized, w.Code)
		// Will likely get internal server error due to missing service implementation
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest}, w.Code)
	})
}

// TestGetSubscriptionEndpoints tests various subscription GET endpoints
func TestGetSubscriptionEndpoints(t *testing.T) {
	server := createTestServer(t)
	defer server.Close(context.Background())

	userID := uuid.New().String()
	email := "test@example.com"
	token := createTestToken(server, userID, email)

	endpoints := []struct {
		name string
		path string
	}{
		{"GetActiveSubscription", "/api/v1/subscriptions/active"},
		{"GetSubscriptionHistory", "/api/v1/subscriptions/history"},
		{"GetUserPurchases", "/api/v1/subscriptions/purchases"},
		{"GetMyBillingStatus", "/api/v1/me/billing-status"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name+"_RequiresAuth", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", endpoint.path, nil)

			server.Handler().ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code,
				"Endpoint %s should require authentication", endpoint.path)
		})

		t.Run(endpoint.name+"_WithAuth", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", endpoint.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)

			server.Handler().ServeHTTP(w, req)

			// Should not be unauthorized with valid token
			assert.NotEqual(t, http.StatusUnauthorized, w.Code,
				"Endpoint %s should accept valid auth", endpoint.path)
			// Will likely get internal server error due to missing service implementation
			assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest}, w.Code)
		})
	}
}

// TestWebhookEndpoints tests the webhook endpoints
func TestWebhookEndpoints(t *testing.T) {
	server := createTestServer(t)
	defer server.Close(context.Background())

	t.Run("CCBillWebhook", func(t *testing.T) {
		webhookData := "eventType=NewSaleSuccess&subscriptionId=123456"

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/webhook/ccbill?eventType=NewSaleSuccess",
			bytes.NewBufferString(webhookData))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		server.Handler().ServeHTTP(w, req)

		// Should not return 404 (route exists)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
		// Will handle webhook processing (success or error)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusForbidden, http.StatusBadRequest}, w.Code)
	})

	t.Run("MobiusWebhook", func(t *testing.T) {
		webhookData := map[string]interface{}{
			"event": "subscription.created",
			"id":    "sub_123456",
		}

		body, _ := json.Marshal(webhookData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/webhook/mobius", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		server.Handler().ServeHTTP(w, req)

		// Should not return 404 (route exists)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
		// Will handle webhook processing (success or error)
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusForbidden, http.StatusBadRequest}, w.Code)
	})

	t.Run("InvalidProcessor", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/webhook/invalid", nil)

		server.Handler().ServeHTTP(w, req)

		// Should handle invalid processor appropriately
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusInternalServerError}, w.Code)
	})
}

// TestHealthEndpoints tests the health check endpoints
func TestHealthEndpoints(t *testing.T) {
	server := createTestServer(t)
	defer server.Close(context.Background())

	t.Run("HealthLive", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health/live", nil)

		server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "ok", response["status"])
		assert.Equal(t, "billing", response["service"])
	})

	t.Run("HealthReady", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health/ready", nil)

		server.Handler().ServeHTTP(w, req)

		// Ready check depends on DB/Redis connectivity
		// In test environment without proper setup, may return 503
		assert.Contains(t, []int{http.StatusOK, http.StatusServiceUnavailable}, w.Code)
	})
}

// TestPaymentMethodEndpoints tests payment method endpoints
func TestPaymentMethodEndpoints(t *testing.T) {
	server := createTestServer(t)
	defer server.Close(context.Background())

	userID := uuid.New().String()
	email := "test@example.com"
	token := createTestToken(server, userID, email)
	_ = token // Used in auth tests below

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/payment-methods"},
		{"DELETE", "/api/v1/payment-methods/123"},
		{"PUT", "/api/v1/payment-methods/123/activate"},
	}

	for _, endpoint := range endpoints {
		t.Run(fmt.Sprintf("%s_%s_RequiresAuth", endpoint.method, endpoint.path), func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(endpoint.method, endpoint.path, nil)

			server.Handler().ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code,
				"Endpoint %s %s should require authentication", endpoint.method, endpoint.path)
		})
	}
}

// TestNotificationEndpoints tests notification endpoints
func TestNotificationEndpoints(t *testing.T) {
	server := createTestServer(t)
	defer server.Close(context.Background())

	userID := uuid.New().String()
	email := "test@example.com"
	token := createTestToken(server, userID, email)
	_ = token // Used in auth tests below

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/notifications"},
		{"GET", "/api/v1/notifications/unread-count"},
		{"POST", "/api/v1/notifications/123/read"},
	}

	for _, endpoint := range endpoints {
		t.Run(fmt.Sprintf("%s_%s_RequiresAuth", endpoint.method, endpoint.path), func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(endpoint.method, endpoint.path, nil)

			server.Handler().ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code,
				"Endpoint %s %s should require authentication", endpoint.method, endpoint.path)
		})
	}
}

// TestFlexFormURL tests the CCBill FlexForm URL generation
func TestFlexFormURL(t *testing.T) {
	server := createTestServer(t)
	defer server.Close(context.Background())

	userID := uuid.New().String()
	email := "test@example.com"
	token := createTestToken(server, userID, email)

	t.Run("FlexFormURL_RequiresAuth", func(t *testing.T) {
		requestData := map[string]interface{}{
			"price_id": uuid.New().String(),
		}

		body, _ := json.Marshal(requestData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		server.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("FlexFormURL_WithAuth", func(t *testing.T) {
		requestData := map[string]interface{}{
			"price_id": uuid.New().String(),
		}

		body, _ := json.Marshal(requestData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/subscriptions/ccbill/flexform-url", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		server.Handler().ServeHTTP(w, req)

		// Should not be unauthorized with valid token
		assert.NotEqual(t, http.StatusUnauthorized, w.Code)
		// Will likely get internal server error due to missing service implementation
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest}, w.Code)
	})
}
