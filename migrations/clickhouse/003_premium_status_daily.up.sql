-- Premium status snapshots for analytics
-- One row per (user_id, day) representing premium status at end of day.
-- This table is owned by doujins-billing because it is derived from billing/subscription state.

CREATE TABLE IF NOT EXISTS premium_status_daily {{ON_CLUSTER}} (
  day Date,
  user_id String,
  is_premium UInt8,
  last_updated DateTime('UTC') DEFAULT now()
) ENGINE = ReplicatedReplacingMergeTree('/clickhouse/tables/{database}/{table}', '{replica}', last_updated)
PARTITION BY toYYYYMM(day)
ORDER BY (day, user_id)
SETTINGS index_granularity = 8192;
