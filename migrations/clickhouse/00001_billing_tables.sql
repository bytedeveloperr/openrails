-- ClickHouse billing tables needed by doujins-billing
-- Matches fields used in internal/services/billing_event_service.go

-- subscription_events
CREATE TABLE IF NOT EXISTS subscription_events (
    event_id UUID,
    subscription_id UUID,
    user_id String,
    event_type LowCardinality(String),
    processor LowCardinality(String),
    processor_subscription_id Nullable(String),
    processor_transaction_id Nullable(String),
    metadata String,
    timestamp DateTime('UTC'),
    created_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (timestamp, user_id, subscription_id);

-- payment_events
CREATE TABLE IF NOT EXISTS payment_events (
    event_id UUID,
    subscription_id Nullable(UUID),
    user_id String,
    event_type LowCardinality(String),
    processor LowCardinality(String),
    processor_transaction_id Nullable(String),
    amount Nullable(Float64),
    currency LowCardinality(String) DEFAULT 'USD',
    billing_info String,
    webhook_source LowCardinality(String),
    metadata String,
    timestamp DateTime('UTC'),
  created_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (timestamp, user_id);

-- webhook_events (incoming webhook processing logs)
CREATE TABLE IF NOT EXISTS webhook_events (
    event_id UUID,
    webhook_source LowCardinality(String),
    event_type String,
    subscription_id Nullable(UUID),
    user_id Nullable(String),
    processor_subscription_id Nullable(String),
    processor_transaction_id Nullable(String),
    processing_status LowCardinality(String),
    processing_time_ms UInt32,
    error_message Nullable(String),
    webhook_payload String,
    headers String,
    timestamp DateTime('UTC'),
    processed_at Nullable(DateTime('UTC')),
    created_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (timestamp, event_id);

-- acu_events (Automatic Card Updater)
CREATE TABLE IF NOT EXISTS acu_events (
    event_id UUID,
    subscription_id Nullable(UUID),
    user_id Nullable(String),
    event_type LowCardinality(String),
    processor LowCardinality(String),
    processor_subscription_id Nullable(String),
    card_info String,
    update_status LowCardinality(String),
    requires_action Bool,
    reason String,
    metadata String,
    timestamp DateTime('UTC'),
    created_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (timestamp, event_id);

-- chargeback_events
CREATE TABLE IF NOT EXISTS chargeback_events (
    event_id UUID,
    chargeback_id String,
    batch_id String,
    subscription_id Nullable(UUID),
    user_id Nullable(String),
    event_type LowCardinality(String),
    processor LowCardinality(String),
    processor_transaction_id Nullable(String),
    amount Nullable(Float64),
    currency LowCardinality(String) DEFAULT 'USD',
    chargeback_type LowCardinality(String),
    reason String,
    status LowCardinality(String),
    metadata String,
    timestamp DateTime('UTC'),
    created_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (timestamp, event_id);
