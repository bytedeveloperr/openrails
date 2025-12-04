-- 006_add_processors_jsonb.up.sql
-- Consolidates processor-specific columns into a single JSONB 'processors' field
-- This enables extensibility for future processors without schema changes

-- Add the new processors JSONB column
ALTER TABLE billing.prices ADD COLUMN IF NOT EXISTS processors JSONB;

-- Drop old processor-specific columns (no data migration needed for fresh start)
ALTER TABLE billing.prices DROP COLUMN IF EXISTS nmi_plan_id;
ALTER TABLE billing.prices DROP COLUMN IF EXISTS nmi_provider;
ALTER TABLE billing.prices DROP COLUMN IF EXISTS ccbill_price_id;

-- Drop old indexes on removed columns (if they exist)
DROP INDEX IF EXISTS billing.idx_prices_nmi_plan_provider;
DROP INDEX IF EXISTS billing.idx_prices_ccbill_price_id;

-- Add GIN index for efficient JSONB queries
CREATE INDEX IF NOT EXISTS idx_prices_processors ON billing.prices USING GIN (processors);

-- Add specific indexes for common processor lookups
CREATE INDEX IF NOT EXISTS idx_prices_nmi_plan_id ON billing.prices ((processors->'nmi'->>'plan_id'))
    WHERE processors->'nmi'->>'plan_id' IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_prices_ccbill_price_id ON billing.prices ((processors->'ccbill'->>'price_id'))
    WHERE processors->'ccbill'->>'price_id' IS NOT NULL;

COMMENT ON COLUMN billing.prices.processors IS 'JSONB map of processor name -> processor config. Keys: nmi, ccbill, solana. Example: {"nmi": {"plan_id": "123", "provider": "mobius"}, "ccbill": {"price_id": "456"}}';
