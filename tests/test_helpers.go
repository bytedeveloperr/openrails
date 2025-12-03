//go:build integration

package tests

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"

	authtesting "github.com/PaulFidika/authkit/testing"
	"github.com/gin-gonic/gin"

	"github.com/doujins-org/doujins-billing/internal/server"
)

var (
	// Test issuer for auth verification (shared across tests)
	testIssuerOnce sync.Once
	testIssuer     *authtesting.TestIssuer

	// Shared test container suite for tests that need infra
	sharedSuiteOnce sync.Once
	sharedSuite     *TestContainerSuite
)

func init() {
	gin.SetMode(gin.TestMode)
}

// getTestIssuer returns a shared test issuer for authentication.
// The issuer provides a JWKS endpoint and can sign tokens.
func getTestIssuer() *authtesting.TestIssuer {
	testIssuerOnce.Do(func() {
		testIssuer = authtesting.NewTestIssuerWithAudience("test-app")
	})
	return testIssuer
}

// GetTestIssuerURL returns the URL of the test JWKS server to use as issuer.
// This is called by testcontainer_suite.go when configuring the auth verifier.
func GetTestIssuerURL() string {
	return getTestIssuer().URL()
}

// getSharedTestSuite returns a shared TestContainerSuite for integration tests.
// The suite is initialized once and reused across tests for performance.
func getSharedTestSuite(t *testing.T) *TestContainerSuite {
	sharedSuiteOnce.Do(func() {
		sharedSuite = NewTestContainerSuite(t)
	})
	return sharedSuite
}

// Helper function to log HTTP response for debugging
func logResponse(t *testing.T, w *httptest.ResponseRecorder, testName string) {
	t.Helper()
	body := w.Body.String()
	if body == "" {
		body = "(empty body)"
	}
	t.Logf("[%s]: Status=%d, Body=%s", testName, w.Code, body)
}

// setupTestServer creates a test server using testcontainers infrastructure.
// This requires the integration build tag and Docker to be available.
func setupTestServer(t *testing.T) *server.Server {
	suite := getSharedTestSuite(t)

	// Register cleanup only once at the end of all tests
	t.Cleanup(func() {
		// Don't cleanup the shared suite here - it will be cleaned up when all tests finish
		// The suite.Cleanup() should be called in TestMain or similar
	})

	return suite.Server
}

// setupTestServerWithAuth creates a test server with a valid JWT token.
// The token is signed by the test issuer and will validate against the JWKS endpoint.
func setupTestServerWithAuth(t *testing.T) (*server.Server, string) {
	srv := setupTestServer(t)
	token := getTestIssuer().CreateToken("test-user-billing-12345", "test@billing.example.com")
	return srv, token
}

// setupTestServerWithRSAuth creates a test server with RS256-authenticated JWT token.
// This is the same as setupTestServerWithAuth since all tokens use RS256.
func setupTestServerWithRSAuth(t *testing.T) (*server.Server, string) {
	srv := setupTestServer(t)
	token := getTestIssuer().CreateToken("test-user-billing-rs256", "rs256@billing.example.com")
	return srv, token
}

// CleanupSharedSuite should be called at the end of all tests to cleanup containers.
func CleanupSharedSuite() {
	if sharedSuite != nil {
		sharedSuite.Server.Close(context.Background())
		sharedSuite.App.Close(context.Background())
		sharedSuite.Cleanup()
	}
	if testIssuer != nil {
		testIssuer.Close()
	}
}
