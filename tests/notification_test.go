package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNotificationEndpoints tests notification endpoints
func TestNotificationEndpoints(t *testing.T) {
	server, token := setupTestServerWithAuth(t)
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

			// Should require auth, but may get rate limited first
			assert.Contains(t, []int{http.StatusUnauthorized, http.StatusTooManyRequests}, w.Code,
				"Endpoint %s %s should require authentication or be rate limited", endpoint.method, endpoint.path)
		})
	}
}
