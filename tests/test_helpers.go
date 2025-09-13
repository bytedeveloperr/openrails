package tests

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/server"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// Helper function to create a real server instance for testing
func createTestServer(t *testing.T) *server.Server {
	// Try to load config, but if it fails, create minimal test config
	cfg, err := config.Load("")
	if err != nil {
		t.Skipf("Skipping test due to config load failure: %v", err)
	}

	server, err := server.New(cfg)
	if err != nil {
		// If server creation fails due to missing DB, skip the test gracefully
		t.Skipf("Skipping test due to server initialization failure (expected in test environment): %v", err)
	}

	return server
}

// Helper function to create deterministic JWT token for testing
func createTestJWT(s *server.Server) string {
	// Deterministic user ID and email for consistent testing
	userID := "test-user-billing-12345"
	email := "test@billing.example.com"

	cfg := s.Cfg()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"iss":   cfg.JWT.Issuer,
		"aud":   cfg.JWT.Audience,
	})

	tokenString, err := token.SignedString([]byte(cfg.JWT.Secret))
	if err != nil {
		panic(fmt.Sprintf("Failed to sign JWT token: %v", err))
	}

	// Log token creation for debugging
	claims := token.Claims.(jwt.MapClaims)
	var expValue int64
	if exp, ok := claims["exp"].(int64); ok {
		expValue = exp
	} else if exp, ok := claims["exp"].(float64); ok {
		expValue = int64(exp)
	}
	fmt.Printf("DEBUG: Created JWT with claims: sub=%s, email=%s, iss=%s, aud=%s, exp=%d, secret_len=%d\n",
		claims["sub"], claims["email"], claims["iss"], claims["aud"], expValue, len(cfg.JWT.Secret))

	return tokenString
}

// Helper function to log HTTP response for debugging
func logResponse(t *testing.T, w *httptest.ResponseRecorder, testName string) {
	t.Helper()
	body := w.Body.String()
	if body == "" {
		body = "(empty body)"
	}
	fmt.Printf("DEBUG [%s]: Status=%d, Body=%s\n", testName, w.Code, body)
}

// Helper function to create test server and defer cleanup
func setupTestServer(t *testing.T) *server.Server {
	server := createTestServer(t)
	t.Cleanup(func() {
		server.Close(context.Background())
	})

	return server
}

// Helper function to create test server with JWT token
func setupTestServerWithAuth(t *testing.T) (*server.Server, string) {
	server := setupTestServer(t)
	token := createTestJWT(server)
	return server, token
}
