-- 00002_seed_billings_tables.sql
-- NOTE: OpenRails should not seed an application-specific catalog (products/prices) in production.
-- This migration only performs safe legacy-data backfills (no new products/prices are inserted).

-- Explicitly set schema to ensure all objects are created in the correct place
-- Set timeouts to prevent hanging migrations
SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- ============================================================================
-- SECTION 0: CLEANUP LEGACY DATA
-- ============================================================================

-- Update any NULL processor_subscription_id values with defaults
UPDATE billing.subscriptions
SET processor_subscription_id = 'LEGACY-' || id::text
WHERE processor_subscription_id IS NULL OR processor_subscription_id = '';

-- Set default started_at for subscriptions that don't have it
UPDATE billing.subscriptions
SET started_at = created_at
WHERE started_at IS NULL;

-- (No production seed data for billing.products or billing.prices)
