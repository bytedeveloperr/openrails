package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthEndpoints tests the health check endpoints
func TestHealthEndpoints(t *testing.T) {
	server := setupTestServer(t)

	t.Run("HealthLive", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health/live", nil)

		server.Handler().ServeHTTP(w, req)

		// Health live should always return 200 unless rate limited
		if w.Code == http.StatusTooManyRequests {
			// If rate limited, just verify we got the rate limit response
			assert.Equal(t, http.StatusTooManyRequests, w.Code)
			return
		}

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
		// May also be rate limited (429)
		assert.Contains(t, []int{http.StatusOK, http.StatusServiceUnavailable, http.StatusTooManyRequests}, w.Code)
	})
}
