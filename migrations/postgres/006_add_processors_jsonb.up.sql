-- 006_add_processors_jsonb.up.sql
-- This migration is now a no-op since the processors JSONB column
-- was added in migration 001_setup_billing_tables.up.sql
--
-- Kept for migration history compatibility - if previously applied,
-- these statements are idempotent (IF NOT EXISTS / IF EXISTS)

-- Ensure processors column exists (no-op if already present)
ALTER TABLE billing.prices ADD COLUMN IF NOT EXISTS processors JSONB;

-- Clean up any legacy columns that might exist from older schemas
ALTER TABLE billing.prices DROP COLUMN IF EXISTS nmi_plan_id;
ALTER TABLE billing.prices DROP COLUMN IF EXISTS nmi_provider;
ALTER TABLE billing.prices DROP COLUMN IF EXISTS ccbill_price_id;

-- Drop old indexes if they exist
DROP INDEX IF EXISTS billing.idx_prices_nmi_plan_provider;
DROP INDEX IF EXISTS billing.idx_prices_ccbill_price_id;

-- Ensure GIN index exists for JSONB queries
CREATE INDEX IF NOT EXISTS idx_prices_processors ON billing.prices USING GIN (processors);
