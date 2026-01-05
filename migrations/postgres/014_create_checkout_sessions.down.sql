-- Migration 014: Checkout sessions

SET lock_timeout = '10s';
SET statement_timeout = '300s';

DROP INDEX IF EXISTS billing.checkout_sessions_expires_at_idx;
DROP INDEX IF EXISTS billing.checkout_sessions_user_status_idx;
DROP INDEX IF EXISTS billing.checkout_sessions_processor_reference_idx;
DROP INDEX IF EXISTS billing.checkout_sessions_processor_transaction_id_idx;
DROP TABLE IF EXISTS billing.checkout_sessions;
