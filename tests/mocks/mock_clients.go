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

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/services/webhook"
)

// MockResponse represents a configurable mock response
type MockResponse struct {
	StatusCode int
	Body       interface{}
	Headers    map[string]string
	Delay      time.Duration
	Error      error
}

// MockCCBillClient provides a mock implementation of CCBill client
type MockCCBillClient struct {
	mu        sync.RWMutex
	responses map[string]*MockResponse
	requests  []MockRequest
	webhooks  []WebhookEvent
	server    *httptest.Server
}

// MockRequest represents a captured request
type MockRequest struct {
	Method    string
	URL       string
	Headers   map[string]string
	Body      string
	Timestamp time.Time
}

// WebhookEvent represents a webhook event to be triggered
type WebhookEvent struct {
	EventType string
	Payload   interface{}
	Delay     time.Duration
}

// NewMockCCBillClient creates a new mock CCBill client
func NewMockCCBillClient() *MockCCBillClient {
	client := &MockCCBillClient{
		responses: make(map[string]*MockResponse),
		requests:  make([]MockRequest, 0),
		webhooks:  make([]WebhookEvent, 0),
	}

	// Set up default responses
	client.SetDefaultResponses()

	return client
}

// SetDefaultResponses sets up default mock responses for CCBill
func (m *MockCCBillClient) SetDefaultResponses() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// FlexForm URL generation
	m.responses["POST /flexforms"] = &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"flexform_url": "https://api.ccbill.com/wap-frontflex/flexforms/test-form-id",
			"form_id":      "test-form-id",
		},
	}

	// DataLink API responses
	m.responses["GET /datalink/subscription"] = &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"subscription_id": "test-ccbill-sub-001",
			"status":          "active",
			"next_bill_date":  time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02"),
		},
	}

	// Cancel subscription
	m.responses["POST /datalink/cancel"] = &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"success": true,
			"message": "Subscription cancelled successfully",
		},
	}
}

// SetResponse sets a mock response for a specific endpoint
func (m *MockCCBillClient) SetResponse(endpoint string, response *MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[endpoint] = response
}

// GetRequests returns all captured requests
func (m *MockCCBillClient) GetRequests() []MockRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]MockRequest(nil), m.requests...)
}

// ClearRequests clears all captured requests
func (m *MockCCBillClient) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = m.requests[:0]
}

// AddWebhookEvent adds a webhook event to be triggered
func (m *MockCCBillClient) AddWebhookEvent(eventType string, payload interface{}, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhooks = append(m.webhooks, WebhookEvent{
		EventType: eventType,
		Payload:   payload,
		Delay:     delay,
	})
}

// TriggerWebhooks sends all queued webhook events to the target URL
func (m *MockCCBillClient) TriggerWebhooks(targetURL string) error {
	m.mu.Lock()
	webhooks := append([]WebhookEvent(nil), m.webhooks...)
	m.webhooks = m.webhooks[:0] // Clear after copying
	m.mu.Unlock()

	for _, webhook := range webhooks {
		if webhook.Delay > 0 {
			time.Sleep(webhook.Delay)
		}

		payloadBytes, err := json.Marshal(webhook.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal webhook payload: %w", err)
		}

		resp, err := http.Post(targetURL, "application/json", strings.NewReader(string(payloadBytes)))
		if err != nil {
			return fmt.Errorf("failed to send webhook: %w", err)
		}
		resp.Body.Close()
	}

	return nil
}

// LoadWebhookPayload loads a webhook payload from testdata and customizes it
func (m *MockCCBillClient) LoadWebhookPayload(eventFile string, customizations map[string]interface{}) (map[string]interface{}, error) {
	payload, err := webhook.LoadTestWebhookPayload("ccbill", eventFile)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Apply customizations
	for key, value := range customizations {
		data[key] = value
	}

	return data, nil
}

// MockMobiusClient provides a mock implementation of Mobius client
type MockMobiusClient struct {
	mu        sync.RWMutex
	responses map[string]*MockResponse
	requests  []MockRequest
	webhooks  []WebhookEvent
}

// NewMockMobiusClient creates a new mock Mobius client
func NewMockMobiusClient() *MockMobiusClient {
	client := &MockMobiusClient{
		responses: make(map[string]*MockResponse),
		requests:  make([]MockRequest, 0),
		webhooks:  make([]WebhookEvent, 0),
	}

	client.SetDefaultResponses()
	return client
}

// SetDefaultResponses sets up default mock responses for Mobius
func (m *MockMobiusClient) SetDefaultResponses() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create subscription
	m.responses["POST /subscriptions"] = &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"subscription_id": "test-mobius-sub-001",
			"status":          "active",
			"plan_id":         "premium_test",
			"amount":          "9.99",
		},
	}

	// Get subscription
	m.responses["GET /subscriptions"] = &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"subscription_id":  "test-mobius-sub-001",
			"status":           "active",
			"next_charge_date": time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02"),
		},
	}

	// Cancel subscription
	m.responses["DELETE /subscriptions"] = &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"success": true,
			"message": "Subscription cancelled successfully",
		},
	}

	// Process payment
	m.responses["POST /payments"] = &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"transaction_id": "test-mobius-txn-001",
			"status":         "approved",
			"amount":         "9.99",
		},
	}
}

// SetResponse sets a mock response for a specific endpoint
func (m *MockMobiusClient) SetResponse(endpoint string, response *MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[endpoint] = response
}

// GetRequests returns all captured requests
func (m *MockMobiusClient) GetRequests() []MockRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]MockRequest(nil), m.requests...)
}

// ClearRequests clears all captured requests
func (m *MockMobiusClient) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = m.requests[:0]
}

// AddWebhookEvent adds a webhook event to be triggered
func (m *MockMobiusClient) AddWebhookEvent(eventType string, payload interface{}, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhooks = append(m.webhooks, WebhookEvent{
		EventType: eventType,
		Payload:   payload,
		Delay:     delay,
	})
}

// TriggerWebhooks sends all queued webhook events to the target URL
func (m *MockMobiusClient) TriggerWebhooks(targetURL string) error {
	m.mu.Lock()
	webhooks := append([]WebhookEvent(nil), m.webhooks...)
	m.webhooks = m.webhooks[:0]
	m.mu.Unlock()

	for _, webhook := range webhooks {
		if webhook.Delay > 0 {
			time.Sleep(webhook.Delay)
		}

		payloadBytes, err := json.Marshal(webhook.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal webhook payload: %w", err)
		}

		resp, err := http.Post(targetURL, "application/json", strings.NewReader(string(payloadBytes)))
		if err != nil {
			return fmt.Errorf("failed to send webhook: %w", err)
		}
		resp.Body.Close()
	}

	return nil
}

// LoadWebhookPayload loads a webhook payload from testdata and customizes it
func (m *MockMobiusClient) LoadWebhookPayload(eventFile string, customizations map[string]interface{}) ([]map[string]interface{}, error) {
	payload, err := webhook.LoadTestWebhookPayload("mobius", eventFile)
	if err != nil {
		return nil, err
	}

	var data []map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Apply customizations to each event in the array
	for i := range data {
		for key, value := range customizations {
			// Handle nested event_body customizations
			if strings.HasPrefix(key, "event_body.") {
				nestedKey := strings.TrimPrefix(key, "event_body.")
				if eventBody, ok := data[i]["event_body"].(map[string]interface{}); ok {
					eventBody[nestedKey] = value
				}
			} else {
				data[i][key] = value
			}
		}
	}

	return data, nil
}

// MockSolanaClient provides a mock implementation of Solana client
type MockSolanaClient struct {
	mu           sync.RWMutex
	responses    map[string]*MockResponse
	requests     []MockRequest
	transactions map[string]*MockSolanaTransaction
}

// MockSolanaTransaction represents a mock Solana transaction
type MockSolanaTransaction struct {
	Signature     string
	Status        string
	Amount        float64
	FromAddress   string
	ToAddress     string
	TokenMint     string
	Confirmations int
	Timestamp     time.Time
}

// NewMockSolanaClient creates a new mock Solana client
func NewMockSolanaClient() *MockSolanaClient {
	client := &MockSolanaClient{
		responses:    make(map[string]*MockResponse),
		requests:     make([]MockRequest, 0),
		transactions: make(map[string]*MockSolanaTransaction),
	}

	client.SetDefaultResponses()
	return client
}

// SetDefaultResponses sets up default mock responses for Solana
func (m *MockSolanaClient) SetDefaultResponses() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get balance
	m.responses["POST /getBalance"] = &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"result": map[string]interface{}{
				"value": 1000000000, // 1 SOL in lamports
			},
		},
	}

	// Get transaction
	m.responses["POST /getTransaction"] = &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"result": map[string]interface{}{
				"meta": map[string]interface{}{
					"err": nil,
				},
				"transaction": map[string]interface{}{
					"signatures": []string{"test-signature-001"},
				},
			},
		},
	}

	// Send transaction
	m.responses["POST /sendTransaction"] = &MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"result": "test-signature-001",
		},
	}
}

// SetResponse sets a mock response for a specific RPC method
func (m *MockSolanaClient) SetResponse(method string, response *MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[fmt.Sprintf("POST /%s", method)] = response
}

// AddTransaction adds a mock transaction
func (m *MockSolanaClient) AddTransaction(signature string, transaction *MockSolanaTransaction) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transactions[signature] = transaction
}

// GetTransaction returns a mock transaction by signature
func (m *MockSolanaClient) GetTransaction(signature string) (*MockSolanaTransaction, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tx, exists := m.transactions[signature]
	return tx, exists
}

// GetRequests returns all captured requests
func (m *MockSolanaClient) GetRequests() []MockRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]MockRequest(nil), m.requests...)
}

// ClearRequests clears all captured requests
func (m *MockSolanaClient) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = m.requests[:0]
}

// SimulatePayment simulates a Solana payment transaction
func (m *MockSolanaClient) SimulatePayment(fromAddress, toAddress string, amount float64, tokenMint string) *MockSolanaTransaction {
	signature := fmt.Sprintf("mock-signature-%d", time.Now().UnixNano())

	transaction := &MockSolanaTransaction{
		Signature:     signature,
		Status:        "confirmed",
		Amount:        amount,
		FromAddress:   fromAddress,
		ToAddress:     toAddress,
		TokenMint:     tokenMint,
		Confirmations: 1,
		Timestamp:     time.Now(),
	}

	m.AddTransaction(signature, transaction)
	return transaction
}

// WebhookEventGenerator provides utilities for generating webhook events
type WebhookEventGenerator struct {
	ccbillClient *MockCCBillClient
	mobiusClient *MockMobiusClient
}

// NewWebhookEventGenerator creates a new webhook event generator
func NewWebhookEventGenerator(ccbillClient *MockCCBillClient, mobiusClient *MockMobiusClient) *WebhookEventGenerator {
	return &WebhookEventGenerator{
		ccbillClient: ccbillClient,
		mobiusClient: mobiusClient,
	}
}

// GenerateCCBillEvent generates a CCBill webhook event with customizations
func (w *WebhookEventGenerator) GenerateCCBillEvent(eventType string, customizations map[string]interface{}) (map[string]interface{}, error) {
	eventFileMap := map[string]string{
		"NewSaleSuccess":   "newsalesuccess.json",
		"RenewalSuccess":   "renewalsuccess.json",
		"Cancellation":     "cancellation.json",
		"NewSaleFailure":   "newsalefailure.json",
		"RenewalFailure":   "renewalfailure.json",
		"Refund":           "refund.json",
		"Chargeback":       "chargeback.json",
		"Expiration":       "expiration.json",
		"Void":             "void.json",
		"UpgradeSuccess":   "upgradesuccess.json",
		"UpgradeFailure":   "upgradefailure.json",
		"UserReactivation": "userreactivation.json",
	}

	eventFile, exists := eventFileMap[eventType]
	if !exists {
		return nil, fmt.Errorf("unknown CCBill event type: %s", eventType)
	}

	return w.ccbillClient.LoadWebhookPayload(eventFile, customizations)
}

// GenerateMobiusEvent generates a Mobius webhook event with customizations
func (w *WebhookEventGenerator) GenerateMobiusEvent(eventType string, customizations map[string]interface{}) ([]map[string]interface{}, error) {
	eventFileMap := map[string]string{
		"recurring.subscription.add":    "recurring_subscription_add.json",
		"recurring.subscription.update": "recurring_subscription_update.json",
		"recurring.subscription.delete": "recurring_subscription_delete.json",
	}

	eventFile, exists := eventFileMap[eventType]
	if !exists {
		return nil, fmt.Errorf("unknown Mobius event type: %s", eventType)
	}

	return w.mobiusClient.LoadWebhookPayload(eventFile, customizations)
}

// ValidateWebhookPayload validates a webhook payload structure
func (w *WebhookEventGenerator) ValidateWebhookPayload(processor models.Processor, eventFile string) error {
	return webhook.ValidateEvent(string(processor), eventFile)
}

// ValidateAllWebhookPayloads validates all webhook payloads for a processor
func (w *WebhookEventGenerator) ValidateAllWebhookPayloads(processor models.Processor) error {
	return webhook.ValidateAllEvents(string(processor))
}
