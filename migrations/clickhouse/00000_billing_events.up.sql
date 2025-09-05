-- =============================================================================
-- Combined ClickHouse Analytics Migration (Final Schema)
-- =============================================================================

-- 1) gallery_view_events (final columns; avoid Nullable in ORDER BY)
CREATE TABLE IF NOT EXISTS gallery_view_events (
    view_id UUID,
    gallery_id UInt64,
    user_id Nullable(String),
    session_id Nullable(String),
    language LowCardinality(String) DEFAULT 'en',
    timestamp DateTime('UTC'),
    time_spent UInt32,
    unique_pages_viewed UInt32,
    max_page_reached UInt32,
    total_pages UInt32,
    last_page_name LowCardinality(Nullable(String)),
    entry_source LowCardinality(String),
    exit_type LowCardinality(String),
    engagement_score UInt8,
    is_return_visit Bool,
    created_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (gallery_id, timestamp)
SETTINGS index_granularity = 8192;

-- evolve older tables to final shape
ALTER TABLE gallery_view_events DROP COLUMN IF EXISTS pages_viewed;
ALTER TABLE gallery_view_events ADD COLUMN IF NOT EXISTS language LowCardinality(String) DEFAULT 'en' AFTER session_id;
ALTER TABLE gallery_view_events ADD COLUMN IF NOT EXISTS last_page_name LowCardinality(Nullable(String)) AFTER total_pages;

CREATE INDEX IF NOT EXISTS idx_gallery_view_events_user_ts 
ON gallery_view_events (user_id, timestamp) TYPE minmax GRANULARITY 1;
CREATE INDEX IF NOT EXISTS idx_gallery_view_events_gallery_ts 
ON gallery_view_events (gallery_id, timestamp) TYPE minmax GRANULARITY 1;
CREATE INDEX IF NOT EXISTS idx_gallery_view_events_score 
ON gallery_view_events (engagement_score) TYPE minmax GRANULARITY 1;

-- 2) gallery_interactions
CREATE TABLE IF NOT EXISTS gallery_interactions (
    interaction_id UUID,
    gallery_id UInt64,
    user_id String,
    interaction_type LowCardinality(String),
    timestamp DateTime('UTC'),
    created_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (user_id, gallery_id, timestamp)
SETTINGS index_granularity = 8192;

CREATE INDEX IF NOT EXISTS idx_gallery_interactions_type 
ON gallery_interactions (interaction_type) TYPE set(100) GRANULARITY 1;

-- 3) search tables (no filters_used; avoid Nullable in ORDER BY)
CREATE TABLE IF NOT EXISTS search_queries (
    search_id UUID,
    user_id Nullable(String),
    session_id Nullable(String),
    query_text String,
    result_count UInt32,
    timestamp DateTime('UTC'),
    created_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (timestamp, search_id)
SETTINGS index_granularity = 8192;
ALTER TABLE search_queries DROP COLUMN IF EXISTS filters_used;

CREATE INDEX IF NOT EXISTS idx_search_queries_query 
ON search_queries (query_text) TYPE bloom_filter GRANULARITY 1;

CREATE TABLE IF NOT EXISTS search_clicks (
    search_id UUID,
    gallery_id UInt64,
    result_position UInt16,
    time_to_click UInt32,
    timestamp DateTime('UTC'),
    created_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (search_id, gallery_id, timestamp)
SETTINGS index_granularity = 8192;

-- 4) user_history_current (ReplacingMergeTree(last_updated))
CREATE TABLE IF NOT EXISTS user_history_current (
    user_id String,
    gallery_id UInt64,
    first_viewed_at DateTime('UTC'),
    last_viewed_at DateTime('UTC'),
    total_views UInt32,
    total_time_spent UInt32,
    max_engagement_score UInt8,
    last_engagement_score UInt8,
    last_exit_type LowCardinality(String),
    has_interacted Bool DEFAULT false,
    resume_name LowCardinality(String) DEFAULT '',
    progress_count UInt32 DEFAULT 0,
    progress_total UInt32 DEFAULT 0,
    completed Bool DEFAULT false,
    last_updated DateTime('UTC') DEFAULT now()
) ENGINE = ReplacingMergeTree(last_updated)
PARTITION BY (cityHash64(user_id) % 1000)
ORDER BY (user_id, gallery_id)
SETTINGS index_granularity = 8192;

-- 5) galleries (final; no artist_group_id)
CREATE TABLE IF NOT EXISTS galleries (
    gallery_id UInt64,
    created_at DateTime,
    live_at DateTime,
    updated_at DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY gallery_id
SETTINGS index_granularity = 8192;

-- 6) relationships
CREATE TABLE IF NOT EXISTS gallery_series (
    gallery_id UInt64,
    series_id UUID,
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (gallery_id, series_id);

CREATE TABLE IF NOT EXISTS gallery_tags (
    gallery_id UInt64,
    tag_id UUID,
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (gallery_id, tag_id);

CREATE TABLE IF NOT EXISTS gallery_artists (
    gallery_id UInt64,
    artist_id UUID,
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (gallery_id, artist_id);

CREATE TABLE IF NOT EXISTS gallery_characters (
    gallery_id UInt64,
    character_id UUID,
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (gallery_id, character_id);

-- 7) popularity views (age-normalized, joined to galleries)
CREATE VIEW IF NOT EXISTS v_gallery_popularity_all AS
SELECT 
    gallery_id,
    count() AS view_count,
    uniq(user_id) AS unique_users,
    avg(engagement_score) AS avg_engagement,
    log10(count() + 1) * avg(engagement_score) AS popularity_score
FROM gallery_view_events
GROUP BY gallery_id
ORDER BY popularity_score DESC;

CREATE VIEW IF NOT EXISTS v_gallery_popularity_1d AS
SELECT 
    gve.gallery_id,
    count() AS view_count,
    uniq(gve.user_id) AS unique_users,
    avg(gve.engagement_score) AS avg_engagement,
    greatest(1.0, 24.0 / greatest(1.0, toFloat64(dateDiff('hour', greatest(g.live_at, now() - INTERVAL 1 DAY), now())))) AS age_boost,
    log10(count() + 1) * avg(gve.engagement_score) * greatest(1.0, 24.0 / greatest(1.0, toFloat64(dateDiff('hour', greatest(g.live_at, now() - INTERVAL 1 DAY), now())))) AS popularity_score
FROM gallery_view_events gve
LEFT JOIN galleries g ON gve.gallery_id = g.gallery_id
WHERE gve.timestamp >= now() - INTERVAL 1 DAY
GROUP BY gve.gallery_id, g.live_at
ORDER BY popularity_score DESC;

-- 8) billing tables (final)
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

-- 9) blog visitors + aggregates with TTL
CREATE TABLE IF NOT EXISTS blog_post_visitors (
    post_id UUID,
    post_public_id UInt32,
    visitor_key String,
    user_id Nullable(String),
    first_seen DateTime('UTC'),
    last_seen DateTime('UTC'),
    created_at DateTime('UTC') DEFAULT now(),
    updated_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (post_id, visitor_key)
TTL created_at + INTERVAL 30 DAY;

CREATE TABLE IF NOT EXISTS blog_post_uv_daily (
    post_id UUID,
    day Date,
    uv_state AggregateFunction(uniqState, String)
) ENGINE = AggregatingMergeTree()
ORDER BY (post_id, day);

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_blog_post_uv_daily
TO blog_post_uv_daily AS
SELECT post_id, toDate(first_seen) AS day, uniqState(visitor_key) AS uv_state
FROM blog_post_visitors
GROUP BY post_id, day;

CREATE TABLE IF NOT EXISTS blog_post_uv_all (
    post_id UUID,
    uv_state AggregateFunction(uniqState, String)
) ENGINE = AggregatingMergeTree()
ORDER BY post_id;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_blog_post_uv_all
TO blog_post_uv_all AS
SELECT post_id, uniqState(visitor_key) AS uv_state
FROM blog_post_visitors
GROUP BY post_id;

-- 10) migration tracking (safety; tool also creates this)
CREATE TABLE IF NOT EXISTS clickhouse_migrations (
    migration_name String,
    applied_at DateTime('UTC') DEFAULT now()
) ENGINE = MergeTree()
ORDER BY migration_name;
