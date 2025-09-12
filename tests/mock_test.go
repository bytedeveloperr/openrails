package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockResponse represents a configurable mock response
type MockResponse struct {
	StatusCode int
	Body       interface{}
	Headers    map[string]string
	Delay      time.Duration
	Error      error
}

// SimpleMockClient provides basic mock functionality
type SimpleMockClient struct {
	responses map[string]*MockResponse
	requests  []MockRequest
}

// MockRequest represents a captured request
type MockRequest struct {
	Method    string
	URL       string
	Headers   map[string]string
	Body      string
	Timestamp time.Time
}

// NewSimpleMockClient creates a new simple mock client
func NewSimpleMockClient() *SimpleMockClient {
	return &SimpleMockClient{
		responses: make(map[string]*MockResponse),
		requests:  make([]MockRequest, 0),
	}
}

// SetResponse sets a mock response for a specific endpoint
func (m *SimpleMockClient) SetResponse(endpoint string, response *MockResponse) {
	m.responses[endpoint] = response
}

// GetResponse gets a mock response for an endpoint
func (m *SimpleMockClient) GetResponse(endpoint string) *MockResponse {
	return m.responses[endpoint]
}

// GetRequests returns all captured requests
func (m *SimpleMockClient) GetRequests() []MockRequest {
	return m.requests
}

// ClearRequests clears all captured requests
func (m *SimpleMockClient) ClearRequests() {
	m.requests = m.requests[:0]
}

// SimulateRequest simulates making a request and captures it
func (m *SimpleMockClient) SimulateRequest(method, url, body string) *MockResponse {
	// Capture the request
	request := MockRequest{
		Method:    method,
		URL:       url,
		Body:      body,
		Timestamp: time.Now(),
		Headers:   make(map[string]string),
	}
	m.requests = append(m.requests, request)

	// Return mock response if available
	return m.GetResponse(method + " " + url)
}

// TestMockClient tests the mock client functionality
func TestMockClient(t *testing.T) {
	t.Run("Set and Get Mock Response", func(t *testing.T) {
		client := NewSimpleMockClient()
		mockResponse := &MockResponse{
			StatusCode: 200,
			Body: map[string]interface{}{
				"success": true,
				"message": "Test response",
			},
		}

		client.SetResponse("GET /api/test", mockResponse)

		response := client.GetResponse("GET /api/test")
		require.NotNil(t, response, "Should get mock response")
		assert.Equal(t, 200, response.StatusCode, "Should have correct status code")

		body, ok := response.Body.(map[string]interface{})
		require.True(t, ok, "Should be able to cast body")
		assert.Equal(t, true, body["success"], "Should have correct success value")
		assert.Equal(t, "Test response", body["message"], "Should have correct message")
	})

	t.Run("Simulate Request and Capture", func(t *testing.T) {
		client := NewSimpleMockClient()

		// Set up mock response
		mockResponse := &MockResponse{
			StatusCode: 201,
			Body:       map[string]interface{}{"created": true},
		}
		client.SetResponse("POST /api/create", mockResponse)

		// Simulate request
		response := client.SimulateRequest("POST", "/api/create", `{"name": "test"}`)

		// Check response
		require.NotNil(t, response, "Should get response")
		assert.Equal(t, 201, response.StatusCode, "Should have correct status code")

		// Check captured requests
		requests := client.GetRequests()
		require.Len(t, requests, 1, "Should capture one request")

		request := requests[0]
		assert.Equal(t, "POST", request.Method, "Should capture correct method")
		assert.Equal(t, "/api/create", request.URL, "Should capture correct URL")
		assert.Equal(t, `{"name": "test"}`, request.Body, "Should capture correct body")
		assert.False(t, request.Timestamp.IsZero(), "Should have timestamp")
	})

	t.Run("Multiple Requests", func(t *testing.T) {
		client := NewSimpleMockClient()

		// Simulate multiple requests
		client.SimulateRequest("GET", "/api/users", "")
		client.SimulateRequest("POST", "/api/users", `{"name": "John"}`)
		client.SimulateRequest("PUT", "/api/users/1", `{"name": "Jane"}`)

		requests := client.GetRequests()
		assert.Len(t, requests, 3, "Should capture all requests")

		assert.Equal(t, "GET", requests[0].Method, "First request should be GET")
		assert.Equal(t, "POST", requests[1].Method, "Second request should be POST")
		assert.Equal(t, "PUT", requests[2].Method, "Third request should be PUT")
	})

	t.Run("Clear Requests", func(t *testing.T) {
		client := NewSimpleMockClient()

		// Ensure we have some requests
		client.SimulateRequest("GET", "/api/test", "")
		assert.Len(t, client.GetRequests(), 1, "Should have one request")

		// Clear requests
		client.ClearRequests()
		assert.Len(t, client.GetRequests(), 0, "Should have no requests after clear")
	})

	t.Run("No Mock Response", func(t *testing.T) {
		client := NewSimpleMockClient()
		response := client.SimulateRequest("GET", "/api/nonexistent", "")
		assert.Nil(t, response, "Should return nil for non-existent endpoint")
	})
}

// TestMockResponseSerialization tests JSON serialization of mock responses
func TestMockResponseSerialization(t *testing.T) {
	t.Run("Serialize Mock Response Body", func(t *testing.T) {
		mockResponse := &MockResponse{
			StatusCode: 200,
			Body: map[string]interface{}{
				"id":      123,
				"name":    "Test User",
				"active":  true,
				"balance": 99.99,
			},
		}

		// Test JSON serialization
		bodyBytes, err := json.Marshal(mockResponse.Body)
		require.NoError(t, err, "Should serialize body to JSON")

		var deserializedBody map[string]interface{}
		err = json.Unmarshal(bodyBytes, &deserializedBody)
		require.NoError(t, err, "Should deserialize JSON back")

		assert.Equal(t, float64(123), deserializedBody["id"], "Should preserve ID")
		assert.Equal(t, "Test User", deserializedBody["name"], "Should preserve name")
		assert.Equal(t, true, deserializedBody["active"], "Should preserve active status")
		assert.Equal(t, 99.99, deserializedBody["balance"], "Should preserve balance")
	})

	t.Run("Mock Response with Headers", func(t *testing.T) {
		mockResponse := &MockResponse{
			StatusCode: 201,
			Body:       map[string]interface{}{"created": true},
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"X-Request-ID":  "test-123",
				"Cache-Control": "no-cache",
			},
		}

		assert.Equal(t, "application/json", mockResponse.Headers["Content-Type"], "Should have content type header")
		assert.Equal(t, "test-123", mockResponse.Headers["X-Request-ID"], "Should have request ID header")
		assert.Equal(t, "no-cache", mockResponse.Headers["Cache-Control"], "Should have cache control header")
	})

	t.Run("Mock Response with Delay", func(t *testing.T) {
		mockResponse := &MockResponse{
			StatusCode: 200,
			Body:       map[string]interface{}{"delayed": true},
			Delay:      100 * time.Millisecond,
		}

		assert.Equal(t, 100*time.Millisecond, mockResponse.Delay, "Should have correct delay")
	})
}
