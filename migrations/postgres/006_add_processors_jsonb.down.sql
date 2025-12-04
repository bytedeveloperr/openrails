-- 006_add_processors_jsonb.down.sql
-- Reverts processors JSONB back to individual columns

-- Drop the new indexes
DROP INDEX IF EXISTS billing.idx_prices_processors;
DROP INDEX IF EXISTS billing.idx_prices_nmi_plan_id;
DROP INDEX IF EXISTS billing.idx_prices_ccbill_price_id;

-- Add back the old columns
ALTER TABLE billing.prices ADD COLUMN IF NOT EXISTS nmi_plan_id TEXT;
ALTER TABLE billing.prices ADD COLUMN IF NOT EXISTS nmi_provider TEXT;
ALTER TABLE billing.prices ADD COLUMN IF NOT EXISTS ccbill_price_id TEXT;

-- Drop the processors column
ALTER TABLE billing.prices DROP COLUMN IF EXISTS processors;

-- Recreate original indexes
CREATE INDEX IF NOT EXISTS idx_prices_nmi_plan_provider ON billing.prices(nmi_plan_id, nmi_provider)
    WHERE nmi_plan_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_prices_ccbill_price_id ON billing.prices(ccbill_price_id)
    WHERE ccbill_price_id IS NOT NULL;
