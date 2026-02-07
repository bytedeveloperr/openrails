SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- Make billing.credit_transactions stateful so it can represent holds that are later captured/released/expired.
-- This migration:
-- 1) Adds lifecycle columns to billing.credit_transactions
-- 2) Converts source_id from UUID -> TEXT (so holds can use non-UUID request IDs)
-- 3) Makes balance_after nullable (holds don't change balance)
-- 4) Backfills existing billing.credit_holds into billing.credit_transactions as transaction_type='hold'
-- 5) Merges captured holds with their legacy withdrawal rows (source='hold', source_id=<hold_id>) and deletes the legacy rows
-- 6) Drops billing.credit_holds

-- 1) Add lifecycle fields.
ALTER TABLE billing.credit_transactions
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'posted',
  ADD COLUMN IF NOT EXISTS authorized_amount BIGINT,
  ADD COLUMN IF NOT EXISTS captured_amount BIGINT,
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp;

-- 2) Convert source_id UUID -> TEXT (idempotency keys / request IDs may not be UUIDs).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'billing'
      AND table_name = 'credit_transactions'
      AND column_name = 'source_id'
      AND udt_name = 'uuid'
  ) THEN
    ALTER TABLE billing.credit_transactions
      ALTER COLUMN source_id TYPE TEXT
      USING source_id::text;
  END IF;
END$$;

-- 3) balance_after can be NULL for non-posting transactions (holds).
ALTER TABLE billing.credit_transactions
  ALTER COLUMN balance_after DROP NOT NULL;

-- Ensure updated_at is aligned for historical rows.
UPDATE billing.credit_transactions
  SET updated_at = created_at
WHERE updated_at IS NULL OR updated_at < created_at;

-- 4) Backfill credit_holds -> credit_transactions (as holds).
INSERT INTO billing.credit_transactions (
  id,
  user_id,
  credit_type_id,
  amount,
  balance_after,
  transaction_type,
  source,
  source_id,
  expires_at,
  description,
  created_at,
  status,
  authorized_amount,
  captured_amount,
  updated_at
)
SELECT
  ch.id,
  ch.user_id,
  ch.credit_type_id,
  0,
  NULL,
  'hold',
  ch.source,
  ch.source_id,
  ch.expires_at,
  NULL,
  ch.created_at,
  ch.status,
  ch.amount,
  ch.captured_amount,
  ch.updated_at
FROM billing.credit_holds ch
ON CONFLICT (id) DO NOTHING;

-- 5) Merge captured holds with their legacy withdrawal rows and delete the legacy rows.
-- Legacy capture implementation created a withdrawal row with source='hold' and source_id=<hold_uuid>.
WITH matches AS (
  SELECT
    h.id AS hold_id,
    w.id AS withdrawal_id,
    w.amount AS withdrawal_amount,
    w.balance_after AS withdrawal_balance_after,
    w.description AS withdrawal_description,
    w.created_at AS withdrawal_created_at
  FROM billing.credit_transactions h
  JOIN billing.credit_transactions w
    ON w.transaction_type = 'withdrawal'
   AND w.source = 'hold'
   AND w.source_id = h.id::text
  WHERE h.transaction_type = 'hold'
    AND h.status = 'captured'
)
UPDATE billing.credit_transactions h
SET
  amount = m.withdrawal_amount,
  balance_after = m.withdrawal_balance_after,
  description = COALESCE(h.description, m.withdrawal_description),
  updated_at = GREATEST(h.updated_at, m.withdrawal_created_at)
FROM matches m
WHERE h.id = m.hold_id;

DELETE FROM billing.credit_transactions w
USING billing.credit_transactions h
WHERE h.transaction_type = 'hold'
  AND h.status = 'captured'
  AND w.transaction_type = 'withdrawal'
  AND w.source = 'hold'
  AND w.source_id = h.id::text;

-- 6) Drop old holds table.
DROP TABLE IF EXISTS billing.credit_holds;

-- Idempotency: allow retry-safe holds keyed by (user_id, credit_type_id, source, source_id).
CREATE UNIQUE INDEX IF NOT EXISTS uniq_credit_hold_idem
  ON billing.credit_transactions(user_id, credit_type_id, source, source_id)
  WHERE transaction_type = 'hold';

-- Idempotency for deposits: enforce uniqueness for (user, type, source, source_id) when caller supplies source_id.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_credit_deposit_idem
  ON billing.credit_transactions(user_id, credit_type_id, source, source_id)
  WHERE transaction_type = 'deposit' AND source_id IS NOT NULL;

