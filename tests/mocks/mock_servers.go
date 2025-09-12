//go:build integration

package mocks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// MockServer provides a configurable HTTP mock server
type MockServer struct {
	server    *httptest.Server
	mu        sync.RWMutex
	routes    map[string]*MockResponse
	requests  []MockRequest
	webhookCh chan WebhookEvent
}

// NewMockServer creates a new mock server
func NewMockServer() *MockServer {
	ms := &MockServer{
		routes:    make(map[string]*MockResponse),
		requests:  make([]MockRequest, 0),
		webhookCh: make(chan WebhookEvent, 100),
	}

	ms.server = httptest.NewServer(http.HandlerFunc(ms.handleRequest))
	return ms
}

// URL returns the mock server URL
func (ms *MockServer) URL() string {
	return ms.server.URL
}

// Close closes the mock server
func (ms *MockServer) Close() {
	ms.server.Close()
	close(ms.webhookCh)
}

// SetRoute sets a mock response for a specific route
func (ms *MockServer) SetRoute(method, path string, response *MockResponse) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	key := fmt.Sprintf("%s %s", method, path)
	ms.routes[key] = response
}

// GetRequests returns all captured requests
func (ms *MockServer) GetRequests() []MockRequest {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return append([]MockRequest(nil), ms.requests...)
}

// ClearRequests clears all captured requests
func (ms *MockServer) ClearRequests() {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.requests = ms.requests[:0]
}

// handleRequest handles incoming HTTP requests
func (ms *MockServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Capture the request
	ms.captureRequest(r)

	// Find matching route
	key := fmt.Sprintf("%s %s", r.Method, r.URL.Path)

	ms.mu.RLock()
	response, exists := ms.routes[key]
	ms.mu.RUnlock()

	if !exists {
		// Try to find a pattern match
		response = ms.findPatternMatch(r.Method, r.URL.Path)
	}

	if response == nil {
		http.NotFound(w, r)
		return
	}

	// Apply delay if specified
	if response.Delay > 0 {
		time.Sleep(response.Delay)
	}

	// Return error if specified
	if response.Error != nil {
		http.Error(w, response.Error.Error(), http.StatusInternalServerError)
		return
	}

	// Set headers
	for key, value := range response.Headers {
		w.Header().Set(key, value)
	}

	// Set status code
	w.WriteHeader(response.StatusCode)

	// Write response body
	if response.Body != nil {
		switch body := response.Body.(type) {
		case string:
			w.Write([]byte(body))
		case []byte:
			w.Write(body)
		default:
			json.NewEncoder(w).Encode(body)
		}
	}
}

// captureRequest captures an incoming request for later inspection
func (ms *MockServer) captureRequest(r *http.Request) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	headers := make(map[string]string)
	for key, values := range r.Header {
		headers[key] = strings.Join(values, ", ")
	}

	body := ""
	if r.Body != nil {
		bodyBytes := make([]byte, r.ContentLength)
		r.Body.Read(bodyBytes)
		body = string(bodyBytes)
	}

	request := MockRequest{
		Method:    r.Method,
		URL:       r.URL.String(),
		Headers:   headers,
		Body:      body,
		Timestamp: time.Now(),
	}

	ms.requests = append(ms.requests, request)
}

// findPatternMatch finds a route that matches the request pattern
func (ms *MockServer) findPatternMatch(method, path string) *MockResponse {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	for routeKey, response := range ms.routes {
		parts := strings.SplitN(routeKey, " ", 2)
		if len(parts) != 2 {
			continue
		}

		routeMethod, routePath := parts[0], parts[1]

		if routeMethod == method && ms.pathMatches(routePath, path) {
			return response
		}
	}

	return nil
}

// pathMatches checks if a route path matches the request path (supports wildcards)
func (ms *MockServer) pathMatches(routePath, requestPath string) bool {
	// Simple wildcard matching
	if strings.Contains(routePath, "*") {
		prefix := strings.Split(routePath, "*")[0]
		return strings.HasPrefix(requestPath, prefix)
	}

	return routePath == requestPath
}

// MobiusMockServer provides a specialized mock server for Mobius API
type MobiusMockServer struct {
	*MockServer
	webhookURL string
}

// NewMobiusMockServer creates a new Mobius mock server
func NewMobiusMockServer() *MobiusMockServer {
	ms := &MobiusMockServer{
		MockServer: NewMockServer(),
	}

	ms.setupMobiusRoutes()
	return ms
}

// SetWebhookURL sets the webhook URL for sending events
func (ms *MobiusMockServer) SetWebhookURL(url string) {
	ms.webhookURL = url
}

// setupMobiusRoutes sets up default Mobius API routes
func (ms *MobiusMockServer) setupMobiusRoutes() {
	// Create subscription
	ms.SetRoute("POST", "/api/subscriptions", &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"subscription_id": "mock-mobius-sub-001",
			"status":          "active",
			"plan_id":         "premium_test",
		},
	})

	// Get subscription
	ms.SetRoute("GET", "/api/subscriptions/*", &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"subscription_id":  "mock-mobius-sub-001",
			"status":           "active",
			"next_charge_date": time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02"),
		},
	})

	// Cancel subscription
	ms.SetRoute("DELETE", "/api/subscriptions/*", &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"success": true,
			"message": "Subscription cancelled",
		},
	})

	// Process payment
	ms.SetRoute("POST", "/api/payments", &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"transaction_id": "mock-mobius-txn-001",
			"status":         "approved",
		},
	})
}

// TriggerWebhook sends a webhook event to the configured webhook URL
func (ms *MobiusMockServer) TriggerWebhook(eventType string, payload interface{}) error {
	if ms.webhookURL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	resp, err := http.Post(ms.webhookURL, "application/json", strings.NewReader(string(payloadBytes)))
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// CCBillMockServer provides a specialized mock server for CCBill API
type CCBillMockServer struct {
	*MockServer
	webhookURL string
}

// NewCCBillMockServer creates a new CCBill mock server
func NewCCBillMockServer() *CCBillMockServer {
	ms := &CCBillMockServer{
		MockServer: NewMockServer(),
	}

	ms.setupCCBillRoutes()
	return ms
}

// SetWebhookURL sets the webhook URL for sending events
func (ms *CCBillMockServer) SetWebhookURL(url string) {
	ms.webhookURL = url
}

// setupCCBillRoutes sets up default CCBill API routes
func (ms *CCBillMockServer) setupCCBillRoutes() {
	// FlexForm generation
	ms.SetRoute("POST", "/wap-frontflex/flexforms/*", &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"flexform_url": "https://api.ccbill.com/wap-frontflex/flexforms/test-form-id",
			"form_id":      "test-form-id",
		},
	})

	// DataLink - Get subscription
	ms.SetRoute("GET", "/api/datalink/subscription-details", &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"subscription_id": "mock-ccbill-sub-001",
			"status":          "active",
			"next_bill_date":  time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02"),
		},
	})

	// DataLink - Cancel subscription
	ms.SetRoute("POST", "/api/datalink/void-subscription", &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"success": true,
			"message": "Subscription cancelled",
		},
	})
}

// TriggerWebhook sends a webhook event to the configured webhook URL
func (ms *CCBillMockServer) TriggerWebhook(eventType string, payload interface{}) error {
	if ms.webhookURL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	resp, err := http.Post(ms.webhookURL, "application/json", strings.NewReader(string(payloadBytes)))
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// SolanaMockServer provides a specialized mock server for Solana RPC
type SolanaMockServer struct {
	*MockServer
	transactions map[string]*MockSolanaTransaction
	mu           sync.RWMutex
}

// NewSolanaMockServer creates a new Solana RPC mock server
func NewSolanaMockServer() *SolanaMockServer {
	ms := &SolanaMockServer{
		MockServer:   NewMockServer(),
		transactions: make(map[string]*MockSolanaTransaction),
	}

	ms.setupSolanaRoutes()
	return ms
}

// setupSolanaRoutes sets up default Solana RPC routes
func (ms *SolanaMockServer) setupSolanaRoutes() {
	// All Solana RPC calls are POST requests
	ms.SetRoute("POST", "/", &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  nil, // Will be customized per method
		},
	})
}

// AddTransaction adds a mock transaction
func (ms *SolanaMockServer) AddTransaction(signature string, transaction *MockSolanaTransaction) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.transactions[signature] = transaction
}

// GetTransaction returns a mock transaction
func (ms *SolanaMockServer) GetTransaction(signature string) (*MockSolanaTransaction, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	tx, exists := ms.transactions[signature]
	return tx, exists
}

// MockServiceManager manages all mock services for integration testing
type MockServiceManager struct {
	CCBillServer *CCBillMockServer
	MobiusServer *MobiusMockServer
	SolanaServer *SolanaMockServer

	CCBillClient *MockCCBillClient
	MobiusClient *MockMobiusClient
	SolanaClient *MockSolanaClient

	WebhookGenerator *WebhookEventGenerator
}

// NewMockServiceManager creates a new mock service manager
func NewMockServiceManager() *MockServiceManager {
	ccbillClient := NewMockCCBillClient()
	mobiusClient := NewMockMobiusClient()
	solanaClient := NewMockSolanaClient()

	return &MockServiceManager{
		CCBillServer: NewCCBillMockServer(),
		MobiusServer: NewMobiusMockServer(),
		SolanaServer: NewSolanaMockServer(),

		CCBillClient: ccbillClient,
		MobiusClient: mobiusClient,
		SolanaClient: solanaClient,

		WebhookGenerator: NewWebhookEventGenerator(ccbillClient, mobiusClient),
	}
}

// SetupWebhookURLs configures webhook URLs for all mock servers
func (msm *MockServiceManager) SetupWebhookURLs(baseURL string) {
	msm.CCBillServer.SetWebhookURL(fmt.Sprintf("%s/api/v1/webhooks/ccbill", baseURL))
	msm.MobiusServer.SetWebhookURL(fmt.Sprintf("%s/api/v1/webhooks/mobius", baseURL))
}

// Cleanup closes all mock servers
func (msm *MockServiceManager) Cleanup() {
	if msm.CCBillServer != nil {
		msm.CCBillServer.Close()
	}
	if msm.MobiusServer != nil {
		msm.MobiusServer.Close()
	}
	if msm.SolanaServer != nil {
		msm.SolanaServer.Close()
	}
}

// ClearAllRequests clears requests from all mock services
func (msm *MockServiceManager) ClearAllRequests() {
	msm.CCBillClient.ClearRequests()
	msm.MobiusClient.ClearRequests()
	msm.SolanaClient.ClearRequests()

	msm.CCBillServer.ClearRequests()
	msm.MobiusServer.ClearRequests()
	msm.SolanaServer.ClearRequests()
}
