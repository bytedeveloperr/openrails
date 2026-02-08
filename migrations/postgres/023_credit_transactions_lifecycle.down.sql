SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- Best-effort revert. This does NOT fully restore the previous semantics, but it recreates
-- billing.credit_holds and removes lifecycle columns/indexes.

-- Recreate credit_holds table (matches the schema from 008_add_credits_tables).
CREATE TABLE IF NOT EXISTS billing.credit_holds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    credit_type_id UUID NOT NULL REFERENCES billing.credit_types(id),
    amount BIGINT NOT NULL,
    source TEXT NOT NULL,
    source_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active', -- active|captured|released|expired
    expires_at TIMESTAMPTZ NOT NULL,
    captured_amount BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_credit_holds_user_status ON billing.credit_holds(user_id, credit_type_id, status);
CREATE INDEX IF NOT EXISTS idx_credit_holds_expires ON billing.credit_holds(expires_at);

-- Backfill holds from credit_transactions.
INSERT INTO billing.credit_holds (id, user_id, credit_type_id, amount, source, source_id, status, expires_at, captured_amount, created_at, updated_at)
SELECT
  ct.id,
  ct.user_id::uuid,
  ct.credit_type_id,
  ct.authorized_amount,
  ct.source,
  ct.source_id,
  ct.status,
  COALESCE(ct.expires_at, current_timestamp),
  ct.captured_amount,
  ct.created_at,
  ct.updated_at
FROM billing.credit_transactions ct
WHERE ct.transaction_type = 'hold'
  AND ct.authorized_amount IS NOT NULL
ON CONFLICT (id) DO NOTHING;

-- Drop indexes added by the up migration.
DROP INDEX IF EXISTS billing.uniq_credit_hold_idem;
DROP INDEX IF EXISTS billing.uniq_credit_deposit_idem;

-- Remove lifecycle columns (best-effort).
ALTER TABLE billing.credit_transactions
  DROP COLUMN IF EXISTS status,
  DROP COLUMN IF EXISTS authorized_amount,
  DROP COLUMN IF EXISTS captured_amount,
  DROP COLUMN IF EXISTS updated_at;

-- Revert balance_after to NOT NULL by setting NULLs to 0.
UPDATE billing.credit_transactions
  SET balance_after = COALESCE(balance_after, 0)
WHERE balance_after IS NULL;

ALTER TABLE billing.credit_transactions
  ALTER COLUMN balance_after SET NOT NULL;

-- source_id: best-effort convert TEXT -> UUID (NULL if not parseable).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'billing'
      AND table_name = 'credit_transactions'
      AND column_name = 'source_id'
      AND data_type = 'text'
  ) THEN
    ALTER TABLE billing.credit_transactions
      ALTER COLUMN source_id TYPE UUID
      USING NULLIF(source_id, '')::uuid;
  END IF;
END$$;

