package services

import (
    "context"
    "encoding/json"
    "fmt"
    "regexp"
    "strings"
    "time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/doujins-org/doujins-billing/config"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// BillingEventService handles logging billing events to ClickHouse
type BillingEventService struct {
    clickhouseConn driver.Conn
    config         *config.ClickHouseConfig
}

// NewBillingEventService creates a new billing event service
func NewBillingEventService(cfg *config.ClickHouseConfig) (*BillingEventService, error) {
    // Feature gate: presence of URL indicates intent to use CH
    if cfg == nil || cfg.ServerURL == "" {
        log.Warn("ClickHouse URL not configured - billing events will not be logged")
        return &BillingEventService{}, nil
    }

    // Try connect; if it fails, return service with nil conn so it can retry later
    conn, err := initClickHouseConnection(cfg)
    if err != nil {
        log.WithError(err).Warn("ClickHouse unavailable at startup; will retry on use")
        return &BillingEventService{clickhouseConn: nil, config: cfg}, nil
    }

    return &BillingEventService{
        clickhouseConn: conn,
        config:         cfg,
    }, nil
}

// ensureConn lazily (re)establishes the ClickHouse connection
func (s *BillingEventService) ensureConn(ctx context.Context) error {
    if s.config == nil || s.config.ServerURL == "" {
        return fmt.Errorf("ClickHouse URL not configured")
    }
    if s.clickhouseConn != nil {
        // Quick ping; if it fails, reopen
        pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
        defer cancel()
        if err := s.clickhouseConn.Ping(pingCtx); err == nil {
            return nil
        }
        _ = s.clickhouseConn.Close()
        s.clickhouseConn = nil
    }
    conn, err := initClickHouseConnection(s.config)
    if err != nil {
        return err
    }
    s.clickhouseConn = conn
    return nil
}

// Close closes the ClickHouse connection
func (s *BillingEventService) Close() error {
	if s.clickhouseConn != nil {
		return s.clickhouseConn.Close()
	}
	return nil
}

// SubscriptionEventData represents data for subscription events
type SubscriptionEventData struct {
    EventID                 uuid.UUID `json:"event_id"`
    SubscriptionID          uuid.UUID `json:"subscription_id"`
    UserID                  uuid.UUID `json:"user_id"`
    EventType               string    `json:"event_type"`
    Processor               string    `json:"processor"`
    ProcessorSubscriptionID *string   `json:"processor_subscription_id,omitempty"`
    ProcessorTransactionID  *string   `json:"processor_transaction_id,omitempty"`
    Metadata                string    `json:"metadata"`
    Timestamp               time.Time `json:"timestamp"`
}

// PaymentEventData represents data for payment events
type PaymentEventData struct {
	EventID                uuid.UUID  `json:"event_id"`
	SubscriptionID         *uuid.UUID `json:"subscription_id,omitempty"`
	UserID                 uuid.UUID  `json:"user_id"`
	EventType              string     `json:"event_type"`
	Processor              string     `json:"processor"`
	ProcessorTransactionID *string    `json:"processor_transaction_id,omitempty"`
	Amount                 *float64   `json:"amount,omitempty"`
	Currency               string     `json:"currency"`
	BillingInfo            string     `json:"billing_info"`
	WebhookSource          string     `json:"webhook_source"`
	Metadata               string     `json:"metadata"`
	Timestamp              time.Time  `json:"timestamp"`
}

// WebhookEventData represents data for webhook events
type WebhookEventData struct {
	EventID                 uuid.UUID  `json:"event_id"`
	WebhookSource           string     `json:"webhook_source"`
	EventType               string     `json:"event_type"`
	SubscriptionID          *uuid.UUID `json:"subscription_id,omitempty"`
	UserID                  *uuid.UUID `json:"user_id,omitempty"`
	ProcessorSubscriptionID *string    `json:"processor_subscription_id,omitempty"`
	ProcessorTransactionID  *string    `json:"processor_transaction_id,omitempty"`
	ProcessingStatus        string     `json:"processing_status"`
	ProcessingTimeMs        uint32     `json:"processing_time_ms"`
	ErrorMessage            *string    `json:"error_message,omitempty"`
	WebhookPayload          string     `json:"webhook_payload"`
	Headers                 string     `json:"headers"`
	Timestamp               time.Time  `json:"timestamp"`
	ProcessedAt             *time.Time `json:"processed_at,omitempty"`
}

// TransactionEventData represents data for transaction events
type TransactionEventData struct {
	EventID                uuid.UUID  `json:"event_id"`
	TransactionID          string     `json:"transaction_id"`
	SubscriptionID         *uuid.UUID `json:"subscription_id,omitempty"`
	UserID                 *uuid.UUID `json:"user_id,omitempty"`
	EventType              string     `json:"event_type"`
	Processor              string     `json:"processor"`
	ProcessorTransactionID *string    `json:"processor_transaction_id,omitempty"`
	Amount                 *float64   `json:"amount,omitempty"`
	Currency               string     `json:"currency"`
	Status                 string     `json:"status"`
	Metadata               string     `json:"metadata"`
	Timestamp              time.Time  `json:"timestamp"`
}

// ACUEventData represents data for Automatic Card Updater events
type ACUEventData struct {
	EventID                 uuid.UUID  `json:"event_id"`
	SubscriptionID          *uuid.UUID `json:"subscription_id,omitempty"`
	UserID                  *uuid.UUID `json:"user_id,omitempty"`
	EventType               string     `json:"event_type"`
	Processor               string     `json:"processor"`
	ProcessorSubscriptionID *string    `json:"processor_subscription_id,omitempty"`
	CardInfo                string     `json:"card_info"`
	UpdateStatus            string     `json:"update_status"`
	RequiresAction          bool       `json:"requires_action"`
	Reason                  string     `json:"reason"`
	Metadata                string     `json:"metadata"`
	Timestamp               time.Time  `json:"timestamp"`
}

// ChargebackEventData represents data for chargeback events
type ChargebackEventData struct {
	EventID                uuid.UUID  `json:"event_id"`
	ChargebackID           string     `json:"chargeback_id"`
	BatchID                string     `json:"batch_id"`
	SubscriptionID         *uuid.UUID `json:"subscription_id,omitempty"`
	UserID                 *uuid.UUID `json:"user_id,omitempty"`
	EventType              string     `json:"event_type"`
	Processor              string     `json:"processor"`
	ProcessorTransactionID *string    `json:"processor_transaction_id,omitempty"`
	Amount                 *float64   `json:"amount,omitempty"`
	Currency               string     `json:"currency"`
	ChargebackType         string     `json:"chargeback_type"`
	Reason                 string     `json:"reason"`
	Status                 string     `json:"status"`
	Metadata               string     `json:"metadata"`
	Timestamp              time.Time  `json:"timestamp"`
}


// Helper: return pointer int value or zero
func valueOrZero(v *int) int {
    if v == nil { return 0 }
    return *v
}

// Helper: return pointer string value or default
func stringOrDefault(v *string, d string) string {
    if v == nil || *v == "" { return d }
    return *v
}

// Basic PII redaction for free-form strings
// - masks emails, long digit sequences, and truncates to a max length
func redactPII(s string) string {
    return redactAndTruncate(s, 1024)
}

func redactAndTruncateJSON(s string, max int) string {
    return redactAndTruncate(s, max)
}

func redactAndTruncate(s string, max int) string {
    // Simple, safe redactions (best-effort)
    // Email-like patterns
    emailRe := regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
    s = emailRe.ReplaceAllString(s, "[redacted-email]")
    // PAN-like long digit sequences (13-19 digits)
    panRe := regexp.MustCompile(`\b\d{13,19}\b`)
    s = panRe.ReplaceAllString(s, "[redacted-digits]")
    if len(s) > max {
        s = s[:max]
    }
    return s
}

// LogSubscriptionEvent logs a subscription lifecycle event
func (s *BillingEventService) LogSubscriptionEvent(ctx context.Context, data SubscriptionEventData) error {
    if err := s.ensureConn(ctx); err != nil {
        log.WithError(err).Debug("ClickHouse not available - skipping subscription event logging")
        return nil
    }

    if data.EventID == uuid.Nil {
        data.EventID = uuid.New()
    }
    if data.Timestamp.IsZero() {
        data.Timestamp = time.Now().UTC()
    }

    query := `
        INSERT INTO subscription_events (
            event_id, subscription_id, user_id, event_type, processor,
            processor_subscription_id, processor_transaction_id,
            metadata, timestamp
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `

    err := s.clickhouseConn.Exec(ctx, query,
        data.EventID,
        data.SubscriptionID,
        data.UserID,
        data.EventType,
        data.Processor,
        data.ProcessorSubscriptionID,
        data.ProcessorTransactionID,
        data.Metadata,
        data.Timestamp,
    )
    if err != nil {
        log.WithError(err).WithFields(log.Fields{
            "event_id":        data.EventID,
            "subscription_id": data.SubscriptionID,
            "event_type":      data.EventType,
            "processor":       data.Processor,
        }).Error("Failed to log subscription event to ClickHouse")
        return fmt.Errorf("failed to log subscription event: %w", err)
    }

    log.WithFields(log.Fields{
        "event_id":        data.EventID,
        "subscription_id": data.SubscriptionID,
        "event_type":      data.EventType,
        "processor":       data.Processor,
    }).Debug("Logged subscription event to ClickHouse")

    return nil
}

// LogPaymentEvent logs a payment transaction event
func (s *BillingEventService) LogPaymentEvent(ctx context.Context, data PaymentEventData) error {
    if err := s.ensureConn(ctx); err != nil {
        log.WithError(err).Debug("ClickHouse not available - skipping payment event logging")
        return nil
    }

    if data.EventID == uuid.Nil {
        data.EventID = uuid.New()
    }
    if data.Timestamp.IsZero() {
        data.Timestamp = time.Now().UTC()
    }
    if data.Currency == "" {
        data.Currency = "USD"
    }

    query := `
        INSERT INTO payment_events (
            event_id, subscription_id, user_id, event_type, processor,
            processor_transaction_id, amount, currency, billing_info,
            webhook_source, metadata, timestamp
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `

    err := s.clickhouseConn.Exec(ctx, query,
        data.EventID,
        data.SubscriptionID,
        data.UserID,
        data.EventType,
        data.Processor,
        data.ProcessorTransactionID,
        data.Amount,
        data.Currency,
        data.BillingInfo,
        data.WebhookSource,
        data.Metadata,
        data.Timestamp,
    )
    if err != nil {
        log.WithError(err).WithFields(log.Fields{
            "event_id":   data.EventID,
            "user_id":    data.UserID,
            "event_type": data.EventType,
            "processor":  data.Processor,
        }).Error("Failed to log payment event to ClickHouse")
        return fmt.Errorf("failed to log payment event: %w", err)
    }

    log.WithFields(log.Fields{
        "event_id":   data.EventID,
        "user_id":    data.UserID,
        "event_type": data.EventType,
        "processor":  data.Processor,
    }).Debug("Logged payment event to ClickHouse")

    return nil
}

// LogTransactionEvent logs a transaction event
func (s *BillingEventService) LogTransactionEvent(ctx context.Context, data TransactionEventData) error {
    // Map transaction event into payment_events to avoid separate table.
    if err := s.ensureConn(ctx); err != nil {
        log.WithError(err).Debug("ClickHouse not available - skipping transaction→payment mapped log")
        return nil
    }

    if data.EventID == uuid.Nil {
        data.EventID = uuid.New()
    }
    if data.Timestamp.IsZero() {
        data.Timestamp = time.Now().UTC()
    }
    if data.Currency == "" {
        data.Currency = "USD"
    }

    evt := strings.ToLower(data.EventType)
    status := strings.ToLower(data.Status)
    paymentType := "webhook_received"
    switch {
    case strings.Contains(evt, "success") || strings.Contains(status, "complete"):
        paymentType = "charge_success"
    case strings.Contains(evt, "fail") || strings.Contains(status, "fail") || strings.Contains(status, "declin"):
        paymentType = "charge_failed"
    }

    source := "api"
    if !strings.Contains(evt, "manual") {
        source = "webhook"
    }

    if data.UserID == nil {
        log.WithFields(log.Fields{
            "transaction_id": data.TransactionID,
            "event_type":     data.EventType,
        }).Debug("Skipping mapped payment event: missing user_id")
        return nil
    }

    ped := PaymentEventData{
        EventID:                data.EventID,
        SubscriptionID:         data.SubscriptionID,
        UserID:                 *data.UserID,
        EventType:              paymentType,
        Processor:              data.Processor,
        ProcessorTransactionID: data.ProcessorTransactionID,
        Amount:                 data.Amount,
        Currency:               data.Currency,
        BillingInfo:            "{}",
        WebhookSource:          source,
        Metadata:               data.Metadata,
        Timestamp:              data.Timestamp,
    }
    return s.LogPaymentEvent(ctx, ped)
}

// LogACUEvent logs an Automatic Card Updater event
func (s *BillingEventService) LogACUEvent(ctx context.Context, data ACUEventData) error {
    if err := s.ensureConn(ctx); err != nil {
        log.WithError(err).Debug("ClickHouse not available - skipping ACU event logging")
        return nil
    }

	if data.EventID == uuid.Nil {
		data.EventID = uuid.New()
	}
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now().UTC()
	}

    // Redact card_info (writer must not store PANs); keep last4 only if present
    data.CardInfo = redactPII(data.CardInfo)

    query := `
        INSERT INTO acu_events (
            event_id, subscription_id, user_id, event_type, processor,
            processor_subscription_id, card_info, update_status, requires_action,
            reason, metadata, timestamp
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `

	err := s.clickhouseConn.Exec(ctx, query,
		data.EventID,
		data.SubscriptionID,
		data.UserID,
		data.EventType,
		data.Processor,
		data.ProcessorSubscriptionID,
		data.CardInfo,
		data.UpdateStatus,
		data.RequiresAction,
		data.Reason,
		data.Metadata,
		data.Timestamp,
	)

	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"event_id":      data.EventID,
			"event_type":    data.EventType,
			"processor":     data.Processor,
			"update_status": data.UpdateStatus,
		}).Error("Failed to log ACU event to ClickHouse")
		return fmt.Errorf("failed to log ACU event: %w", err)
	}

	log.WithFields(log.Fields{
		"event_id":      data.EventID,
		"event_type":    data.EventType,
		"processor":     data.Processor,
		"update_status": data.UpdateStatus,
	}).Debug("Logged ACU event to ClickHouse")

	return nil
}

// LogChargebackEvent logs a chargeback event
func (s *BillingEventService) LogChargebackEvent(ctx context.Context, data ChargebackEventData) error {
	if s.clickhouseConn == nil {
		log.Debug("ClickHouse not available - skipping chargeback event logging")
		return nil
	}

	if data.EventID == uuid.Nil {
		data.EventID = uuid.New()
	}
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now().UTC()
	}
	if data.Currency == "" {
		data.Currency = "USD"
	}

	query := `
		INSERT INTO chargeback_events (
			event_id, chargeback_id, batch_id, subscription_id, user_id, event_type, processor,
			processor_transaction_id, amount, currency, chargeback_type, reason,
			status, metadata, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	err := s.clickhouseConn.Exec(ctx, query,
		data.EventID,
		data.ChargebackID,
		data.BatchID,
		data.SubscriptionID,
		data.UserID,
		data.EventType,
		data.Processor,
		data.ProcessorTransactionID,
		data.Amount,
		data.Currency,
		data.ChargebackType,
		data.Reason,
		data.Status,
		data.Metadata,
		data.Timestamp,
	)

	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"event_id":        data.EventID,
			"chargeback_id":   data.ChargebackID,
			"event_type":      data.EventType,
			"processor":       data.Processor,
			"chargeback_type": data.ChargebackType,
		}).Error("Failed to log chargeback event to ClickHouse")
		return fmt.Errorf("failed to log chargeback event: %w", err)
	}

	log.WithFields(log.Fields{
		"event_id":        data.EventID,
		"chargeback_id":   data.ChargebackID,
		"event_type":      data.EventType,
		"processor":       data.Processor,
		"chargeback_type": data.ChargebackType,
	}).Debug("Logged chargeback event to ClickHouse")

	return nil
}

// Helper method to create metadata JSON
func CreateMetadataJSON(data map[string]interface{}) string {
	if data == nil {
		return "{}"
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		log.WithError(err).Error("Failed to marshal metadata to JSON")
		return "{}"
	}

	return string(jsonData)
}

// initClickHouseConnection creates a connection to ClickHouse
func initClickHouseConnection(cfg *config.ClickHouseConfig) (driver.Conn, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("ClickHouse server URL not configured")
	}

	// Parse the server URL to extract host and port
	serverURL := strings.TrimPrefix(cfg.ServerURL, "http://")
	serverURL = strings.TrimPrefix(serverURL, "https://")

	// Default port if not specified
	if !strings.Contains(serverURL, ":") {
		serverURL += ":9000" // Default ClickHouse native protocol port
	}

    options := &clickhouse.Options{
        Addr: []string{serverURL},
        Auth: clickhouse.Auth{
            Database: cfg.Database,
            Username: cfg.Username,
            Password: cfg.Password,
        },
        Compression: &clickhouse.Compression{
            Method: clickhouse.CompressionLZ4,
        },
        Settings: clickhouse.Settings{
            "max_execution_time": 60,
        },
        DialTimeout: 30 * time.Second,
    }

	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("failed to open ClickHouse connection: %w", err)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	log.Info("Successfully connected to ClickHouse for billing event logging")
	return conn, nil
}
