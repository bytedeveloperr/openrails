//go:build integration

package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedHandlers_SurfaceSplitting(t *testing.T) {
	srv := setupTestServer(t)
	require.NotNil(t, srv)

	// Full handler should include user + admin + webhooks.
	{
		req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code)
	}
	{
		req := httptest.NewRequest(http.MethodGet, "/v1/products", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code)
	}
	{
		// Admin route exists (will likely be 401 without auth, but must not be 404).
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/metrics/summary", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code)
	}
	{
		// Webhook route exists (will likely be 400 for missing body/provider handling, but must not be 404).
		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/stripe", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code)
	}

	// Split handlers should expose only their intended surfaces.
	{
		// Embedded minimal surfaces should NOT include standalone health endpoints.
		req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		w := httptest.NewRecorder()
		srv.UserHandler().ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	}
	{
		req := httptest.NewRequest(http.MethodGet, "/v1/products", nil)
		w := httptest.NewRecorder()
		srv.UserHandler().ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code)
	}
	{
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/metrics/summary", nil)
		w := httptest.NewRecorder()
		srv.UserHandler().ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	}
	{
		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/stripe", nil)
		w := httptest.NewRecorder()
		srv.WebhookHandler().ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code)
	}
	{
		req := httptest.NewRequest(http.MethodGet, "/v1/products", nil)
		w := httptest.NewRecorder()
		srv.WebhookHandler().ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	}
	{
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/metrics/summary", nil)
		w := httptest.NewRecorder()
		srv.AdminHandler().ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code)
	}
	{
		req := httptest.NewRequest(http.MethodGet, "/v1/products", nil)
		w := httptest.NewRecorder()
		srv.AdminHandler().ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	}
}
