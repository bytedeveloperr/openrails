//go:build integration

package mocks

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/services/webhook"
	"github.com/google/uuid"
)

// WebhookPayloadCustomizer provides utilities for customizing webhook payloads
type WebhookPayloadCustomizer struct {
	processor models.Processor
}

// NewWebhookPayloadCustomizer creates a new webhook payload customizer
func NewWebhookPayloadCustomizer(processor models.Processor) *WebhookPayloadCustomizer {
	return &WebhookPayloadCustomizer{
		processor: processor,
	}
}

// LoadAndCustomizePayload loads a webhook payload and applies customizations
func (w *WebhookPayloadCustomizer) LoadAndCustomizePayload(eventFile string, customizations map[string]interface{}) (interface{}, error) {
	payload, err := webhook.LoadTestWebhookPayload(string(w.processor), eventFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load webhook payload: %w", err)
	}

	switch w.processor {
	case models.ProcessorCCBill:
		return w.customizeCCBillPayload(payload, customizations)
	case models.ProcessorMobius:
		return w.customizeMobiusPayload(payload, customizations)
	default:
		return nil, fmt.Errorf("unsupported processor: %s", w.processor)
	}
}

// customizeCCBillPayload customizes a CCBill webhook payload
func (w *WebhookPayloadCustomizer) customizeCCBillPayload(payload string, customizations map[string]interface{}) (map[string]interface{}, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CCBill payload: %w", err)
	}

	// Apply customizations
	for key, value := range customizations {
		data[key] = value
	}

	return data, nil
}

// customizeMobiusPayload customizes a Mobius webhook payload
func (w *WebhookPayloadCustomizer) customizeMobiusPayload(payload string, customizations map[string]interface{}) ([]map[string]interface{}, error) {
	var data []map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Mobius payload: %w", err)
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

// WebhookScenarioBuilder helps build complete webhook test scenarios
type WebhookScenarioBuilder struct {
	processor    models.Processor
	customizer   *WebhookPayloadCustomizer
	user         *TestUser
	subscription *models.Subscription
	price        *models.Price
}

// NewWebhookScenarioBuilder creates a new webhook scenario builder
func NewWebhookScenarioBuilder(processor models.Processor) *WebhookScenarioBuilder {
	return &WebhookScenarioBuilder{
		processor:  processor,
		customizer: NewWebhookPayloadCustomizer(processor),
	}
}

// WithUser sets the user for the webhook scenario
func (w *WebhookScenarioBuilder) WithUser(user *TestUser) *WebhookScenarioBuilder {
	w.user = user
	return w
}

// WithSubscription sets the subscription for the webhook scenario
func (w *WebhookScenarioBuilder) WithSubscription(subscription *models.Subscription) *WebhookScenarioBuilder {
	w.subscription = subscription
	return w
}

// WithPrice sets the price for the webhook scenario
func (w *WebhookScenarioBuilder) WithPrice(price *models.Price) *WebhookScenarioBuilder {
	w.price = price
	return w
}

// BuildCCBillNewSaleSuccess builds a CCBill NewSaleSuccess webhook payload
func (w *WebhookScenarioBuilder) BuildCCBillNewSaleSuccess() (map[string]interface{}, error) {
	if w.user == nil || w.subscription == nil || w.price == nil {
		return nil, fmt.Errorf("user, subscription, and price must be set")
	}

	customizations := map[string]interface{}{
		"email":                      w.user.Email,
		"subscriptionId":             w.subscription.ProcessorSubscriptionID,
		"transactionId":              fmt.Sprintf("ccbill-txn-%s", uuid.New().String()[:8]),
		"subscriptionInitialPrice":   fmt.Sprintf("%.2f", w.price.Amount),
		"subscriptionRecurringPrice": fmt.Sprintf("%.2f", w.price.Amount),
		"accountingInitialPrice":     w.price.Amount,
		"accountingRecurringPrice":   w.price.Amount,
		"billedInitialPrice":         fmt.Sprintf("%.2f", w.price.Amount),
		"billedRecurringPrice":       fmt.Sprintf("%.2f", w.price.Amount),
		"timestamp":                  time.Now().Format("2006-01-02 15:04:05"),
		"nextRenewalDate":            time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02"),
	}

	if w.price.BillingCycleDays != nil {
		customizations["initialPeriod"] = *w.price.BillingCycleDays
		customizations["recurringPeriod"] = *w.price.BillingCycleDays
	}

	return w.customizer.LoadAndCustomizePayload("newsalesuccess.json", customizations)
}

// BuildCCBillRenewalSuccess builds a CCBill RenewalSuccess webhook payload
func (w *WebhookScenarioBuilder) BuildCCBillRenewalSuccess() (map[string]interface{}, error) {
	if w.user == nil || w.subscription == nil || w.price == nil {
		return nil, fmt.Errorf("user, subscription, and price must be set")
	}

	customizations := map[string]interface{}{
		"email":                      w.user.Email,
		"subscriptionId":             w.subscription.ProcessorSubscriptionID,
		"transactionId":              fmt.Sprintf("ccbill-renewal-%s", uuid.New().String()[:8]),
		"subscriptionRecurringPrice": fmt.Sprintf("%.2f", w.price.Amount),
		"accountingRecurringPrice":   w.price.Amount,
		"billedRecurringPrice":       fmt.Sprintf("%.2f", w.price.Amount),
		"timestamp":                  time.Now().Format("2006-01-02 15:04:05"),
		"nextRenewalDate":            time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02"),
	}

	return w.customizer.LoadAndCustomizePayload("renewalsuccess.json", customizations)
}

// BuildCCBillCancellation builds a CCBill Cancellation webhook payload
func (w *WebhookScenarioBuilder) BuildCCBillCancellation() (map[string]interface{}, error) {
	if w.user == nil || w.subscription == nil {
		return nil, fmt.Errorf("user and subscription must be set")
	}

	customizations := map[string]interface{}{
		"email":          w.user.Email,
		"subscriptionId": w.subscription.ProcessorSubscriptionID,
		"timestamp":      time.Now().Format("2006-01-02 15:04:05"),
	}

	return w.customizer.LoadAndCustomizePayload("cancellation.json", customizations)
}

// BuildMobiusSubscriptionAdd builds a Mobius recurring.subscription.add webhook payload
func (w *WebhookScenarioBuilder) BuildMobiusSubscriptionAdd() ([]map[string]interface{}, error) {
	if w.user == nil || w.subscription == nil || w.price == nil {
		return nil, fmt.Errorf("user, subscription, and price must be set")
	}

	customizations := map[string]interface{}{
		"event_body.billing_address.email": w.user.Email,
		"event_body.subscription_id":       w.subscription.ProcessorSubscriptionID,
		"event_body.plan.amount":           fmt.Sprintf("%.2f", w.price.Amount),
		"event_body.plan.id":               *w.price.MobiusPlanID,
		"event_body.next_charge_date":      time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02"),
	}

	if w.price.BillingCycleDays != nil {
		customizations["event_body.plan.day_frequency"] = *w.price.BillingCycleDays
	}

	result, err := w.customizer.LoadAndCustomizePayload("recurring_subscription_add.json", customizations)
	if err != nil {
		return nil, err
	}

	return result.([]map[string]interface{}), nil
}

// BuildMobiusSubscriptionUpdate builds a Mobius recurring.subscription.update webhook payload
func (w *WebhookScenarioBuilder) BuildMobiusSubscriptionUpdate() ([]map[string]interface{}, error) {
	if w.user == nil || w.subscription == nil || w.price == nil {
		return nil, fmt.Errorf("user, subscription, and price must be set")
	}

	customizations := map[string]interface{}{
		"event_body.billing_address.email": w.user.Email,
		"event_body.subscription_id":       w.subscription.ProcessorSubscriptionID,
		"event_body.plan.amount":           fmt.Sprintf("%.2f", w.price.Amount),
		"event_body.plan.id":               *w.price.MobiusPlanID,
		"event_body.next_charge_date":      time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02"),
	}

	result, err := w.customizer.LoadAndCustomizePayload("recurring_subscription_update.json", customizations)
	if err != nil {
		return nil, err
	}

	return result.([]map[string]interface{}), nil
}

// BuildMobiusSubscriptionDelete builds a Mobius recurring.subscription.delete webhook payload
func (w *WebhookScenarioBuilder) BuildMobiusSubscriptionDelete() ([]map[string]interface{}, error) {
	if w.user == nil || w.subscription == nil {
		return nil, fmt.Errorf("user and subscription must be set")
	}

	customizations := map[string]interface{}{
		"event_body.billing_address.email": w.user.Email,
		"event_body.subscription_id":       w.subscription.ProcessorSubscriptionID,
	}

	result, err := w.customizer.LoadAndCustomizePayload("recurring_subscription_delete.json", customizations)
	if err != nil {
		return nil, err
	}

	return result.([]map[string]interface{}), nil
}

// WebhookValidationFramework provides validation utilities for webhook payloads
type WebhookValidationFramework struct{}

// NewWebhookValidationFramework creates a new webhook validation framework
func NewWebhookValidationFramework() *WebhookValidationFramework {
	return &WebhookValidationFramework{}
}

// ValidatePayloadStructure validates the structure of a webhook payload
func (w *WebhookValidationFramework) ValidatePayloadStructure(processor models.Processor, eventType string, payload interface{}) error {
	switch processor {
	case models.ProcessorCCBill:
		return w.validateCCBillPayload(eventType, payload)
	case models.ProcessorMobius:
		return w.validateMobiusPayload(eventType, payload)
	default:
		return fmt.Errorf("unsupported processor: %s", processor)
	}
}

// validateCCBillPayload validates a CCBill webhook payload structure
func (w *WebhookValidationFramework) validateCCBillPayload(eventType string, payload interface{}) error {
	data, ok := payload.(map[string]interface{})
	if !ok {
		return fmt.Errorf("CCBill payload must be a JSON object")
	}

	// Common required fields for all CCBill events
	requiredFields := []string{"timestamp", "clientAccnum", "clientSubacc"}

	// Event-specific required fields
	switch eventType {
	case "NewSaleSuccess", "RenewalSuccess":
		requiredFields = append(requiredFields, "subscriptionId", "transactionId", "email")
	case "Cancellation", "Expiration":
		requiredFields = append(requiredFields, "subscriptionId", "email")
	case "Refund", "Chargeback", "Void":
		requiredFields = append(requiredFields, "transactionId", "subscriptionId")
	}

	// Validate required fields
	for _, field := range requiredFields {
		if _, exists := data[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	return nil
}

// validateMobiusPayload validates a Mobius webhook payload structure
func (w *WebhookValidationFramework) validateMobiusPayload(eventType string, payload interface{}) error {
	data, ok := payload.([]map[string]interface{})
	if !ok {
		return fmt.Errorf("Mobius payload must be an array of JSON objects")
	}

	if len(data) == 0 {
		return fmt.Errorf("Mobius payload array cannot be empty")
	}

	// Validate each event in the array
	for i, event := range data {
		// Required top-level fields
		requiredFields := []string{"event_id", "event_type", "event_body"}
		for _, field := range requiredFields {
			if _, exists := event[field]; !exists {
				return fmt.Errorf("event %d missing required field: %s", i, field)
			}
		}

		// Validate event_body structure
		eventBody, ok := event["event_body"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("event %d event_body must be a JSON object", i)
		}

		// Event-specific validation
		switch eventType {
		case "recurring.subscription.add", "recurring.subscription.update":
			bodyRequiredFields := []string{"subscription_id", "plan", "billing_address"}
			for _, field := range bodyRequiredFields {
				if _, exists := eventBody[field]; !exists {
					return fmt.Errorf("event %d event_body missing required field: %s", i, field)
				}
			}
		case "recurring.subscription.delete":
			if _, exists := eventBody["subscription_id"]; !exists {
				return fmt.Errorf("event %d event_body missing required field: subscription_id", i)
			}
		}
	}

	return nil
}

// ValidateAllEventTypes validates all event types for a processor
func (w *WebhookValidationFramework) ValidateAllEventTypes(processor models.Processor) error {
	return webhook.ValidateAllEvents(string(processor))
}

// ValidateEventFile validates a specific event file
func (w *WebhookValidationFramework) ValidateEventFile(processor models.Processor, eventFile string) error {
	return webhook.ValidateEvent(string(processor), eventFile)
}

// WebhookTestScenario represents a complete webhook test scenario
type WebhookTestScenario struct {
	Processor    models.Processor
	EventType    string
	User         *TestUser
	Subscription *models.Subscription
	Price        *models.Price
	Payload      interface{}
	Customizer   *WebhookPayloadCustomizer
	Builder      *WebhookScenarioBuilder
	Validator    *WebhookValidationFramework
}

// NewWebhookTestScenario creates a new webhook test scenario
func NewWebhookTestScenario(processor models.Processor, eventType string) *WebhookTestScenario {
	return &WebhookTestScenario{
		Processor:  processor,
		EventType:  eventType,
		Customizer: NewWebhookPayloadCustomizer(processor),
		Builder:    NewWebhookScenarioBuilder(processor),
		Validator:  NewWebhookValidationFramework(),
	}
}

// WithTestData sets the test data for the scenario
func (w *WebhookTestScenario) WithTestData(user *TestUser, subscription *models.Subscription, price *models.Price) *WebhookTestScenario {
	w.User = user
	w.Subscription = subscription
	w.Price = price

	w.Builder.WithUser(user).WithSubscription(subscription).WithPrice(price)

	return w
}

// BuildPayload builds the webhook payload for the scenario
func (w *WebhookTestScenario) BuildPayload() error {
	var err error

	switch w.Processor {
	case models.ProcessorCCBill:
		w.Payload, err = w.buildCCBillPayload()
	case models.ProcessorMobius:
		w.Payload, err = w.buildMobiusPayload()
	default:
		return fmt.Errorf("unsupported processor: %s", w.Processor)
	}

	return err
}

// buildCCBillPayload builds a CCBill webhook payload
func (w *WebhookTestScenario) buildCCBillPayload() (interface{}, error) {
	switch w.EventType {
	case "NewSaleSuccess":
		return w.Builder.BuildCCBillNewSaleSuccess()
	case "RenewalSuccess":
		return w.Builder.BuildCCBillRenewalSuccess()
	case "Cancellation":
		return w.Builder.BuildCCBillCancellation()
	default:
		return nil, fmt.Errorf("unsupported CCBill event type: %s", w.EventType)
	}
}

// buildMobiusPayload builds a Mobius webhook payload
func (w *WebhookTestScenario) buildMobiusPayload() (interface{}, error) {
	switch w.EventType {
	case "recurring.subscription.add":
		return w.Builder.BuildMobiusSubscriptionAdd()
	case "recurring.subscription.update":
		return w.Builder.BuildMobiusSubscriptionUpdate()
	case "recurring.subscription.delete":
		return w.Builder.BuildMobiusSubscriptionDelete()
	default:
		return nil, fmt.Errorf("unsupported Mobius event type: %s", w.EventType)
	}
}

// ValidatePayload validates the built payload
func (w *WebhookTestScenario) ValidatePayload() error {
	if w.Payload == nil {
		return fmt.Errorf("payload not built yet, call BuildPayload() first")
	}

	return w.Validator.ValidatePayloadStructure(w.Processor, w.EventType, w.Payload)
}

// GetPayloadJSON returns the payload as JSON string
func (w *WebhookTestScenario) GetPayloadJSON() (string, error) {
	if w.Payload == nil {
		return "", fmt.Errorf("payload not built yet, call BuildPayload() first")
	}

	payloadBytes, err := json.Marshal(w.Payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	return string(payloadBytes), nil
}
