-- Add generic metadata JSONB columns for E2E/test tracing and future extensibility.
-- Safe to run multiple times.

ALTER TABLE billing.payment_methods
  ADD COLUMN IF NOT EXISTS metadata JSONB;

ALTER TABLE billing.payments
  ADD COLUMN IF NOT EXISTS metadata JSONB;

