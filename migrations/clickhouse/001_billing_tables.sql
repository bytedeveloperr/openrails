-- ClickHouse billing tables needed by doujins-billing
-- Matches fields used in internal/services/billing_event_service.go

-- subscription_events
CREATE TABLE IF NOT EXISTS subscription_events ON CLUSTER doujins (
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
) ENGINE = ReplacingMergeTree()
ORDER BY (event_id)
SETTINGS index_granularity = 8192;

-- Data-skipping indexes to speed time/user filters
ALTER TABLE subscription_events ON CLUSTER doujins
  ADD INDEX IF NOT EXISTS idx_subscription_events_ts (timestamp) TYPE minmax GRANULARITY 1,
  ADD INDEX IF NOT EXISTS idx_subscription_events_user (user_id) TYPE set(0) GRANULARITY 1;

-- payment_events
CREATE TABLE IF NOT EXISTS payment_events ON CLUSTER doujins (
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
) ENGINE = ReplacingMergeTree()
ORDER BY (event_id)
SETTINGS index_granularity = 8192;

ALTER TABLE payment_events ON CLUSTER doujins
  ADD INDEX IF NOT EXISTS idx_payment_events_ts (timestamp) TYPE minmax GRANULARITY 1,
  ADD INDEX IF NOT EXISTS idx_payment_events_user (user_id) TYPE set(0) GRANULARITY 1;

-- webhook_events (incoming webhook processing logs)
CREATE TABLE IF NOT EXISTS webhook_events ON CLUSTER doujins (
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
) ENGINE = ReplacingMergeTree()
ORDER BY (event_id)
SETTINGS index_granularity = 8192;

ALTER TABLE webhook_events ON CLUSTER doujins
  ADD INDEX IF NOT EXISTS idx_webhook_events_ts (timestamp) TYPE minmax GRANULARITY 1;

-- acu_events (Automatic Card Updater)
CREATE TABLE IF NOT EXISTS acu_events ON CLUSTER doujins (
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
) ENGINE = ReplacingMergeTree()
ORDER BY (event_id)
SETTINGS index_granularity = 8192;

ALTER TABLE acu_events ON CLUSTER doujins
  ADD INDEX IF NOT EXISTS idx_acu_events_ts (timestamp) TYPE minmax GRANULARITY 1;

-- chargeback_events
CREATE TABLE IF NOT EXISTS chargeback_events ON CLUSTER doujins (
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
) ENGINE = ReplacingMergeTree()
ORDER BY (event_id)
SETTINGS index_granularity = 8192;

ALTER TABLE chargeback_events ON CLUSTER doujins
  ADD INDEX IF NOT EXISTS idx_chargeback_events_ts (timestamp) TYPE minmax GRANULARITY 1;
