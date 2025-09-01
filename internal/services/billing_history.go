package services

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"time"

// 	"github.com/google/uuid"
// )

// // BillingEvent represents a billing event from ClickHouse
// type BillingEvent struct {
// 	EventID                 string                 `json:"event_id"`
// 	SubscriptionID          *string                `json:"subscription_id,omitempty"`
// 	UserID                  string                 `json:"user_id"`
// 	EventType               string                 `json:"event_type"`
// 	Processor               string                 `json:"processor"`
// 	ProcessorSubscriptionID *string                `json:"processor_subscription_id,omitempty"`
// 	ProcessorTransactionID  *string                `json:"processor_transaction_id,omitempty"`
// 	Amount                  *float64               `json:"amount,omitempty"`
// 	Currency                string                 `json:"currency"`
// 	BillingInfo             map[string]interface{} `json:"billing_info,omitempty"`
// 	WebhookSource           *string                `json:"webhook_source,omitempty"`
// 	Metadata                map[string]interface{} `json:"metadata,omitempty"`
// 	Timestamp               time.Time              `json:"timestamp"`
// 	CreatedAt               time.Time              `json:"created_at"`
// }

// // SubscriptionEvent represents a subscription lifecycle event from ClickHouse
// type SubscriptionEvent struct {
// 	EventID                 string                 `json:"event_id"`
// 	SubscriptionID          string                 `json:"subscription_id"`
// 	UserID                  string                 `json:"user_id"`
// 	EventType               string                 `json:"event_type"`
// 	Processor               string                 `json:"processor"`
// 	ProcessorSubscriptionID *string                `json:"processor_subscription_id,omitempty"`
// 	ProcessorTransactionID  *string                `json:"processor_transaction_id,omitempty"`
// 	Amount                  *float64               `json:"amount,omitempty"`
// 	Currency                string                 `json:"currency"`
// 	Metadata                map[string]interface{} `json:"metadata,omitempty"`
// 	Timestamp               time.Time              `json:"timestamp"`
// 	CreatedAt               time.Time              `json:"created_at"`
// }

// // BillingHistoryFilter represents filters for billing history queries
// type BillingHistoryFilter struct {
// 	StartDate *time.Time
// 	EndDate   *time.Time
// 	Processor *string
// 	MinAmount *float64
// 	MaxAmount *float64
// }

// // BillingHistoryStats represents aggregated billing statistics
// type BillingHistoryStats struct {
// 	TotalCharged       float64                   `json:"total_charged"`
// 	TotalCharges       int                       `json:"total_charges"`
// 	SuccessfulCharges  int                       `json:"successful_charges"`
// 	FailedCharges      int                       `json:"failed_charges"`
// 	Refunds            int                       `json:"refunds"`
// 	TotalRefunded      float64                   `json:"total_refunded"`
// 	FirstChargeDate    *time.Time                `json:"first_charge_date,omitempty"`
// 	LastChargeDate     *time.Time                `json:"last_charge_date,omitempty"`
// 	ProcessorBreakdown map[string]ProcessorStats `json:"processor_breakdown"`
// }

// // ProcessorStats represents statistics for a specific payment processor
// type ProcessorStats struct {
// 	TotalCharged      float64 `json:"total_charged"`
// 	TotalCharges      int     `json:"total_charges"`
// 	SuccessfulCharges int     `json:"successful_charges"`
// 	FailedCharges     int     `json:"failed_charges"`
// }

// // BillingHistoryService provides billing history data from ClickHouse
// type BillingHistoryService struct {
// 	// clickHouseClient *analytics.ClickHouseClient // TODO: Re-enable when analytics package is available
// }

// // NewBillingHistoryService creates a new billing history service
// func NewBillingHistoryService() *BillingHistoryService {
// 	return &BillingHistoryService{}
// }

// // GetPaymentHistory retrieves payment history for a user from ClickHouse
// func (s *BillingHistoryService) GetPaymentHistory(ctx context.Context, userID uuid.UUID, filter *BillingHistoryFilter, page, pageSize int) ([]BillingEvent, int64, error) {
// 	if s.clickHouseClient == nil {
// 		return nil, 0, fmt.Errorf("ClickHouse client not available")
// 	}

// 	// Build WHERE clause
// 	whereClause := fmt.Sprintf("user_id = '%s'", userID.String())

// 	if filter != nil {
// 		if filter.StartDate != nil {
// 			whereClause += fmt.Sprintf(" AND timestamp >= '%s'", filter.StartDate.Format("2006-01-02 15:04:05"))
// 		}
// 		if filter.EndDate != nil {
// 			whereClause += fmt.Sprintf(" AND timestamp <= '%s'", filter.EndDate.Format("2006-01-02 15:04:05"))
// 		}
// 		if filter.Processor != nil {
// 			whereClause += fmt.Sprintf(" AND processor = '%s'", *filter.Processor)
// 		}
// 		if filter.MinAmount != nil {
// 			whereClause += fmt.Sprintf(" AND amount >= %.2f", *filter.MinAmount)
// 		}
// 		if filter.MaxAmount != nil {
// 			whereClause += fmt.Sprintf(" AND amount <= %.2f", *filter.MaxAmount)
// 		}
// 	}

// 	// Get total count
// 	countQuery := fmt.Sprintf(`
// 		SELECT COUNT(*) as count
// 		FROM payment_events
// 		WHERE %s
// 	`, whereClause)

// 	countResult, err := s.clickHouseClient.QueryJSON(ctx, countQuery)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to count payment events: %w", err)
// 	}

// 	var totalCount int64
// 	if len(countResult) > 0 {
// 		if countVal, ok := countResult[0]["count"]; ok {
// 			if countInt, ok := countVal.(float64); ok {
// 				totalCount = int64(countInt)
// 			}
// 		}
// 	}

// 	// Get paginated results
// 	offset := (page - 1) * pageSize
// 	dataQuery := fmt.Sprintf(`
// 		SELECT
// 			event_id,
// 			subscription_id,
// 			user_id,
// 			event_type,
// 			processor,
// 			processor_transaction_id,
// 			amount,
// 			currency,
// 			billing_info,
// 			webhook_source,
// 			metadata,
// 			timestamp,
// 			created_at
// 		FROM payment_events
// 		WHERE %s
// 		ORDER BY timestamp DESC
// 		LIMIT %d OFFSET %d
// 	`, whereClause, pageSize, offset)

// 	results, err := s.clickHouseClient.QueryJSON(ctx, dataQuery)
// 	if err != nil {
// 		return nil, 0, fmt.Errorf("failed to query payment events: %w", err)
// 	}

// 	var events []BillingEvent
// 	for _, row := range results {
// 		event := BillingEvent{}

// 		if eventID, ok := row["event_id"].(string); ok {
// 			event.EventID = eventID
// 		}
// 		if subscriptionID, ok := row["subscription_id"].(string); ok && subscriptionID != "" {
// 			event.SubscriptionID = &subscriptionID
// 		}
// 		if userIDStr, ok := row["user_id"].(string); ok {
// 			event.UserID = userIDStr
// 		}
// 		if eventType, ok := row["event_type"].(string); ok {
// 			event.EventType = eventType
// 		}
// 		if processor, ok := row["processor"].(string); ok {
// 			event.Processor = processor
// 		}
// 		if transactionID, ok := row["processor_transaction_id"].(string); ok && transactionID != "" {
// 			event.ProcessorTransactionID = &transactionID
// 		}
// 		if amount, ok := row["amount"].(float64); ok {
// 			event.Amount = &amount
// 		}
// 		if currency, ok := row["currency"].(string); ok {
// 			event.Currency = currency
// 		}
// 		if billingInfoStr, ok := row["billing_info"].(string); ok && billingInfoStr != "" {
// 			var billingInfo map[string]interface{}
// 			if err := json.Unmarshal([]byte(billingInfoStr), &billingInfo); err == nil {
// 				event.BillingInfo = billingInfo
// 			}
// 		}
// 		if webhookSource, ok := row["webhook_source"].(string); ok && webhookSource != "" {
// 			event.WebhookSource = &webhookSource
// 		}
// 		if metadataStr, ok := row["metadata"].(string); ok && metadataStr != "" {
// 			var metadata map[string]interface{}
// 			if err := json.Unmarshal([]byte(metadataStr), &metadata); err == nil {
// 				event.Metadata = metadata
// 			}
// 		}
// 		if timestampStr, ok := row["timestamp"].(string); ok {
// 			if timestamp, err := time.Parse("2006-01-02 15:04:05", timestampStr); err == nil {
// 				event.Timestamp = timestamp
// 			}
// 		}
// 		if createdAtStr, ok := row["created_at"].(string); ok {
// 			if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
// 				event.CreatedAt = createdAt
// 			}
// 		}

// 		events = append(events, event)
// 	}

// 	return events, totalCount, nil
// }

// // GetSubscriptionHistory retrieves subscription history for a user from ClickHouse
// func (s *BillingHistoryService) GetSubscriptionHistory(ctx context.Context, userID uuid.UUID, filter *BillingHistoryFilter) ([]SubscriptionEvent, error) {
// 	if s.clickHouseClient == nil {
// 		return nil, fmt.Errorf("ClickHouse client not available")
// 	}

// 	// Build WHERE clause
// 	whereClause := fmt.Sprintf("user_id = '%s'", userID.String())

// 	if filter != nil {
// 		if filter.StartDate != nil {
// 			whereClause += fmt.Sprintf(" AND timestamp >= '%s'", filter.StartDate.Format("2006-01-02 15:04:05"))
// 		}
// 		if filter.EndDate != nil {
// 			whereClause += fmt.Sprintf(" AND timestamp <= '%s'", filter.EndDate.Format("2006-01-02 15:04:05"))
// 		}
// 		if filter.Processor != nil {
// 			whereClause += fmt.Sprintf(" AND processor = '%s'", *filter.Processor)
// 		}
// 	}

// 	query := fmt.Sprintf(`
// 		SELECT
// 			event_id,
// 			subscription_id,
// 			user_id,
// 			event_type,
// 			processor,
// 			processor_subscription_id,
// 			processor_transaction_id,
// 			amount,
// 			currency,
// 			metadata,
// 			timestamp,
// 			created_at
// 		FROM subscription_events
// 		WHERE %s
// 		ORDER BY timestamp DESC
// 		LIMIT 1000
// 	`, whereClause)

// 	results, err := s.clickHouseClient.QueryJSON(ctx, query)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to query subscription events: %w", err)
// 	}

// 	var events []SubscriptionEvent
// 	for _, row := range results {
// 		event := SubscriptionEvent{}

// 		if eventID, ok := row["event_id"].(string); ok {
// 			event.EventID = eventID
// 		}
// 		if subscriptionID, ok := row["subscription_id"].(string); ok {
// 			event.SubscriptionID = subscriptionID
// 		}
// 		if userIDStr, ok := row["user_id"].(string); ok {
// 			event.UserID = userIDStr
// 		}
// 		if eventType, ok := row["event_type"].(string); ok {
// 			event.EventType = eventType
// 		}
// 		if processor, ok := row["processor"].(string); ok {
// 			event.Processor = processor
// 		}
// 		if subscriptionIDStr, ok := row["processor_subscription_id"].(string); ok && subscriptionIDStr != "" {
// 			event.ProcessorSubscriptionID = &subscriptionIDStr
// 		}
// 		if transactionID, ok := row["processor_transaction_id"].(string); ok && transactionID != "" {
// 			event.ProcessorTransactionID = &transactionID
// 		}
// 		if amount, ok := row["amount"].(float64); ok {
// 			event.Amount = &amount
// 		}
// 		if currency, ok := row["currency"].(string); ok {
// 			event.Currency = currency
// 		}
// 		if metadataStr, ok := row["metadata"].(string); ok && metadataStr != "" {
// 			var metadata map[string]interface{}
// 			if err := json.Unmarshal([]byte(metadataStr), &metadata); err == nil {
// 				event.Metadata = metadata
// 			}
// 		}
// 		if timestampStr, ok := row["timestamp"].(string); ok {
// 			if timestamp, err := time.Parse("2006-01-02 15:04:05", timestampStr); err == nil {
// 				event.Timestamp = timestamp
// 			}
// 		}
// 		if createdAtStr, ok := row["created_at"].(string); ok {
// 			if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
// 				event.CreatedAt = createdAt
// 			}
// 		}

// 		events = append(events, event)
// 	}

// 	return events, nil
// }

// // GetBillingStats retrieves aggregated billing statistics for a user
// func (s *Service) GetBillingStats(ctx context.Context, userID uuid.UUID, filter *BillingHistoryFilter) (*BillingHistoryStats, error) {
// 	if s.clickHouseClient == nil {
// 		return nil, fmt.Errorf("ClickHouse client not available")
// 	}

// 	// Build WHERE clause
// 	whereClause := fmt.Sprintf("user_id = '%s'", userID.String())

// 	if filter != nil {
// 		if filter.StartDate != nil {
// 			whereClause += fmt.Sprintf(" AND timestamp >= '%s'", filter.StartDate.Format("2006-01-02 15:04:05"))
// 		}
// 		if filter.EndDate != nil {
// 			whereClause += fmt.Sprintf(" AND timestamp <= '%s'", filter.EndDate.Format("2006-01-02 15:04:05"))
// 		}
// 	}

// 	// Get overall statistics
// 	statsQuery := fmt.Sprintf(`
// 		SELECT
// 			COUNT(*) as total_charges,
// 			SUM(CASE WHEN event_type = 'charge_success' THEN 1 ELSE 0 END) as successful_charges,
// 			SUM(CASE WHEN event_type = 'charge_failed' THEN 1 ELSE 0 END) as failed_charges,
// 			SUM(CASE WHEN event_type = 'refund' THEN 1 ELSE 0 END) as refunds,
// 			SUM(CASE WHEN event_type = 'charge_success' AND amount IS NOT NULL THEN amount ELSE 0 END) as total_charged,
// 			SUM(CASE WHEN event_type = 'refund' AND amount IS NOT NULL THEN amount ELSE 0 END) as total_refunded,
// 			MIN(CASE WHEN event_type = 'charge_success' THEN timestamp ELSE NULL END) as first_charge_date,
// 			MAX(CASE WHEN event_type = 'charge_success' THEN timestamp ELSE NULL END) as last_charge_date
// 		FROM payment_events
// 		WHERE %s
// 	`, whereClause)

// 	statsResult, err := s.clickHouseClient.QueryJSON(ctx, statsQuery)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to query billing stats: %w", err)
// 	}

// 	stats := &BillingHistoryStats{
// 		ProcessorBreakdown: make(map[string]ProcessorStats),
// 	}

// 	if len(statsResult) > 0 {
// 		row := statsResult[0]

// 		if totalCharges, ok := row["total_charges"].(float64); ok {
// 			stats.TotalCharges = int(totalCharges)
// 		}
// 		if successfulCharges, ok := row["successful_charges"].(float64); ok {
// 			stats.SuccessfulCharges = int(successfulCharges)
// 		}
// 		if failedCharges, ok := row["failed_charges"].(float64); ok {
// 			stats.FailedCharges = int(failedCharges)
// 		}
// 		if refunds, ok := row["refunds"].(float64); ok {
// 			stats.Refunds = int(refunds)
// 		}
// 		if totalCharged, ok := row["total_charged"].(float64); ok {
// 			stats.TotalCharged = totalCharged
// 		}
// 		if totalRefunded, ok := row["total_refunded"].(float64); ok {
// 			stats.TotalRefunded = totalRefunded
// 		}
// 		if firstChargeStr, ok := row["first_charge_date"].(string); ok && firstChargeStr != "" {
// 			if firstCharge, err := time.Parse("2006-01-02 15:04:05", firstChargeStr); err == nil {
// 				stats.FirstChargeDate = &firstCharge
// 			}
// 		}
// 		if lastChargeStr, ok := row["last_charge_date"].(string); ok && lastChargeStr != "" {
// 			if lastCharge, err := time.Parse("2006-01-02 15:04:05", lastChargeStr); err == nil {
// 				stats.LastChargeDate = &lastCharge
// 			}
// 		}
// 	}

// 	// Get processor breakdown
// 	processorQuery := fmt.Sprintf(`
// 		SELECT
// 			processor,
// 			COUNT(*) as total_charges,
// 			SUM(CASE WHEN event_type = 'charge_success' THEN 1 ELSE 0 END) as successful_charges,
// 			SUM(CASE WHEN event_type = 'charge_failed' THEN 1 ELSE 0 END) as failed_charges,
// 			SUM(CASE WHEN event_type = 'charge_success' AND amount IS NOT NULL THEN amount ELSE 0 END) as total_charged
// 		FROM payment_events
// 		WHERE %s
// 		GROUP BY processor
// 	`, whereClause)

// 	processorResult, err := s.clickHouseClient.QueryJSON(ctx, processorQuery)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to query processor breakdown: %w", err)
// 	}

// 	for _, row := range processorResult {
// 		if processor, ok := row["processor"].(string); ok {
// 			processorStats := ProcessorStats{}

// 			if totalCharges, ok := row["total_charges"].(float64); ok {
// 				processorStats.TotalCharges = int(totalCharges)
// 			}
// 			if successfulCharges, ok := row["successful_charges"].(float64); ok {
// 				processorStats.SuccessfulCharges = int(successfulCharges)
// 			}
// 			if failedCharges, ok := row["failed_charges"].(float64); ok {
// 				processorStats.FailedCharges = int(failedCharges)
// 			}
// 			if totalCharged, ok := row["total_charged"].(float64); ok {
// 				processorStats.TotalCharged = totalCharged
// 			}

// 			stats.ProcessorBreakdown[processor] = processorStats
// 		}
// 	}

// 	return stats, nil
// }

// // IsClickHouseAvailable checks if ClickHouse is available
// func (s *Service) IsClickHouseAvailable() bool {
// 	return s.clickHouseClient != nil
// }
