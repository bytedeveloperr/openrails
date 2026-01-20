-- Migration 015 down: best-effort reversal

ALTER TABLE billing.payments
DROP COLUMN IF EXISTS discount_metadata;

ALTER TABLE billing.payments
DROP COLUMN IF EXISTS discount_reason;

ALTER TABLE billing.payments
DROP COLUMN IF EXISTS discount_code;

ALTER TABLE billing.payments
DROP COLUMN IF EXISTS list_amount;

-- NOTE: PostgreSQL enum values cannot be removed easily.
-- We keep billing.processor_type value 'manual' even on down migration.

