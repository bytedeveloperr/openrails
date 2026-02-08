SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- Best-effort revert: credit_blocks -> credit_expiry_batches.
-- credit_expiry_batches historically required expires_at NOT NULL; we map NULL to infinity.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'billing'
          AND table_name = 'credit_blocks'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'billing'
          AND table_name = 'credit_expiry_batches'
    ) THEN
        UPDATE billing.credit_blocks
          SET expires_at = 'infinity'::timestamptz
        WHERE expires_at IS NULL;

        ALTER TABLE billing.credit_blocks
          ALTER COLUMN expires_at SET NOT NULL;

        ALTER TABLE billing.credit_blocks RENAME TO credit_expiry_batches;
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
          AND c.relname = 'idx_credit_blocks_user_expires'
    ) AND NOT EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'billing'
          AND c.relname = 'idx_credit_expiry_batches_user_expires'
    ) THEN
        ALTER INDEX billing.idx_credit_blocks_user_expires RENAME TO idx_credit_expiry_batches_user_expires;
    END IF;
END$$;

DROP INDEX IF EXISTS billing.idx_credit_blocks_user_expires_created;

