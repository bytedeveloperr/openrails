package tests

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/app"
	"github.com/doujins-org/doujins-billing/internal/server"
)

const testJWTSecret = "TEST_JWT_SECRET"

var (
	testRSAOnce      sync.Once
	testRSAPrivate   *rsa.PrivateKey
	testRSAPublicPEM string
)

func ensureTestRSAKeys() {
	testRSAOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(fmt.Sprintf("failed to generate test RSA key: %v", err))
		}
		testRSAPrivate = key
		pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal test RSA public key: %v", err))
		}
		testRSAPublicPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}))
	})
}

func init() {
	gin.SetMode(gin.TestMode)
}

// Helper function to create a real server instance for testing
func createTestServer(t *testing.T) (*server.Server, *app.App) {
	// Try to load config, but if it fails, create minimal test config
	cfg, err := config.Load("")
	if err != nil {
		t.Skipf("Skipping test due to config load failure: %v", err)
	}

	// Inject deterministic auth settings so tests do not rely on external configuration
	if cfg.JWT == nil {
		cfg.JWT = &config.JWTConfig{}
	}
	ensureTestRSAKeys()
	cfg.JWT.Secret = testJWTSecret
	cfg.JWT.PublicKeyPEM = testRSAPublicPEM

	application, err := app.Bootstrap(cfg)
	if err != nil {
		t.Skipf("Skipping test due to app bootstrap failure (expected in test environment): %v", err)
	}

	srv, err := server.New(server.Dependencies{
		Config:       application.Config,
		Cache:        application.Cache,
		Runtime:      application.Runtime,
		Redis:        application.RedisClient,
		AuthVerifier: application.AuthVerifier,
	})
	if err != nil {
		t.Skipf("Skipping test due to server initialization failure (expected in test environment): %v", err)
	}

	return srv, application
}

// Helper function to create deterministic HS256 JWT token for testing
func createTestHS256JWT(s *server.Server) string {
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
	fmt.Printf("DEBUG: Created HS256 JWT with claims: sub=%s, email=%s, iss=%s, aud=%s, exp=%d, secret_len=%d\n",
		claims["sub"], claims["email"], claims["iss"], claims["aud"], expValue, len(cfg.JWT.Secret))

	return tokenString
}

// Helper function to create RS256 JWT token using a generated test key
func createTestRS256JWT(s *server.Server) string {
	ensureTestRSAKeys()

	cfg := s.Cfg()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":   "test-user-billing-rs256",
		"email": "rs256@billing.example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"iss":   cfg.JWT.Issuer,
		"aud":   cfg.JWT.Audience,
	})

	signed, err := token.SignedString(testRSAPrivate)
	if err != nil {
		panic(fmt.Sprintf("failed to sign RS256 JWT: %v", err))
	}

	return signed
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
	srv, application := createTestServer(t)
	t.Cleanup(func() {
		srv.Close(context.Background())
		application.Close(context.Background())
	})

	return srv
}

// Helper function to create test server with JWT token
func setupTestServerWithAuth(t *testing.T) (*server.Server, string) {
	server := setupTestServer(t)
	token := createTestHS256JWT(server)
	return server, token
}

// Helper function to create test server with RS256-authenticated JWT token
func setupTestServerWithRSAuth(t *testing.T) (*server.Server, string) {
	server := setupTestServer(t)
	token := createTestRS256JWT(server)
	return server, token
}
