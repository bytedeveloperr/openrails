-- Migration 015: Payments list amount + discount metadata, and "manual" processor

-- 1) Add processor enum value for off-channel/manual purchases
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_type t
        JOIN pg_namespace n ON n.oid = t.typnamespace
        JOIN pg_enum e ON e.enumtypid = t.oid
        WHERE t.typname = 'processor_type' AND n.nspname = 'billing' AND e.enumlabel = 'manual'
    ) THEN
        ALTER TYPE billing.processor_type ADD VALUE 'manual';
    END IF;
END$$;

-- 2) Add list amount + discount metadata to payments.
-- list_amount is the canonical amount of the referenced Price at purchase time.
ALTER TABLE billing.payments
ADD COLUMN IF NOT EXISTS list_amount BIGINT;

ALTER TABLE billing.payments
ADD COLUMN IF NOT EXISTS discount_code TEXT;

ALTER TABLE billing.payments
ADD COLUMN IF NOT EXISTS discount_reason TEXT;

ALTER TABLE billing.payments
ADD COLUMN IF NOT EXISTS discount_metadata JSONB;

-- Backfill: list_amount defaults to abs(amount) for historic records (including refunds).
UPDATE billing.payments
SET list_amount = ABS(amount)
WHERE list_amount IS NULL;

ALTER TABLE billing.payments
ALTER COLUMN list_amount SET NOT NULL;

