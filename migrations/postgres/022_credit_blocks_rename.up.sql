SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- Rename credit_expiry_batches -> credit_blocks.
-- This is a semantic rename: these rows represent immutable "blocks" of credits that can be consumed.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'billing'
          AND table_name = 'credit_expiry_batches'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'billing'
          AND table_name = 'credit_blocks'
    ) THEN
        ALTER TABLE billing.credit_expiry_batches RENAME TO credit_blocks;
    END IF;
END$$;

-- Index rename (best-effort).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'billing'
          AND c.relname = 'idx_credit_expiry_batches_user_expires'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'billing'
          AND c.relname = 'idx_credit_blocks_user_expires'
    ) THEN
        ALTER INDEX billing.idx_credit_expiry_batches_user_expires RENAME TO idx_credit_blocks_user_expires;
    END IF;
END$$;

-- Make expires_at nullable so blocks can represent both permanent and expiring grants.
ALTER TABLE billing.credit_blocks
  ALTER COLUMN expires_at DROP NOT NULL;

-- Helpful index for consumption ordering (expiry first, then created_at).
CREATE INDEX IF NOT EXISTS idx_credit_blocks_user_expires_created
  ON billing.credit_blocks(user_id, credit_type_id, expires_at, created_at);

-- Backfill: create a non-expiring block for the "permanent remainder" so that
-- user_credit_balances.balance becomes a rollup of credit_blocks.remaining_amount.
--
-- permanent_remaining = balance - SUM(remaining_amount of existing blocks)
WITH block_totals AS (
  SELECT user_id, credit_type_id, COALESCE(SUM(remaining_amount), 0) AS blocks_remaining
  FROM billing.credit_blocks
  GROUP BY user_id, credit_type_id
),
remainders AS (
  SELECT ucb.user_id,
         ucb.credit_type_id,
         GREATEST(ucb.balance - COALESCE(bt.blocks_remaining, 0), 0) AS permanent_remaining
  FROM billing.user_credit_balances ucb
  LEFT JOIN block_totals bt
    ON bt.user_id = ucb.user_id
   AND bt.credit_type_id = ucb.credit_type_id
)
INSERT INTO billing.credit_blocks (id, user_id, credit_type_id, original_amount, remaining_amount, expires_at, source_transaction_id, created_at)
SELECT gen_random_uuid(),
       r.user_id,
       r.credit_type_id,
       r.permanent_remaining,
       r.permanent_remaining,
       NULL,
       NULL,
       NOW()
FROM remainders r
WHERE r.permanent_remaining > 0;

