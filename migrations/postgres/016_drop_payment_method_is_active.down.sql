-- Migration 016 down: Restore is_active column

ALTER TABLE billing.payment_methods
ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;
