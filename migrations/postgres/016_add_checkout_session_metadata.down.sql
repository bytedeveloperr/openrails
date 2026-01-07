-- Migration 016: Add metadata to checkout_sessions

SET lock_timeout = '10s';
SET statement_timeout = '300s';

ALTER TABLE IF EXISTS billing.checkout_sessions
    DROP COLUMN IF EXISTS metadata;
