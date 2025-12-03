//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEndpointAuthRequirements tests that protected endpoints require authentication
func TestEndpointAuthRequirements(t *testing.T) {
	server, _ := setupTestServerWithAuth(t)

	// All endpoints that require authentication
	endpoints := []struct {
		method string
		path   string
	}{
		// Notification endpoints
		{"GET", "/v1/notifications"},
		{"GET", "/v1/notifications/unread-count"},
		{"POST", "/v1/notifications/123/read"},
		// Payment method endpoints
		{"GET", "/v1/payment-methods"},
		{"PUT", "/v1/payment-methods/123"},
		{"DELETE", "/v1/payment-methods/123"},
		{"PUT", "/v1/payment-methods/123/activate"},
	}

	for _, endpoint := range endpoints {
		t.Run(fmt.Sprintf("%s_%s_RequiresAuth", endpoint.method, endpoint.path), func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(endpoint.method, endpoint.path, nil)

			server.Handler().ServeHTTP(w, req)

			// Should require auth, but may get rate limited first
			assert.Contains(t, []int{http.StatusUnauthorized, http.StatusTooManyRequests}, w.Code,
				"Endpoint %s %s should require authentication or be rate limited", endpoint.method, endpoint.path)
		})
	}
}
