-- 011_drop_webhook_events_table.up.sql
-- Drops the legacy billing.webhook_events table in favor of River-based async processing

DROP TABLE IF EXISTS billing.webhook_events;
DROP INDEX IF EXISTS billing.idx_webhook_events_status;
DROP INDEX IF EXISTS billing.idx_webhook_events_processor;
DROP INDEX IF EXISTS billing.idx_webhook_events_next_attempt;
DROP INDEX IF EXISTS billing.idx_webhook_events_event_id;
DROP INDEX IF EXISTS billing.idx_webhook_events_processor_event_id_unique;
