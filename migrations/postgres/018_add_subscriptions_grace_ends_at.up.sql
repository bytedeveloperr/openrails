SET lock_timeout = '10s';
SET statement_timeout = '300s';

ALTER TABLE billing.subscriptions
  ADD COLUMN IF NOT EXISTS grace_ends_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_subscriptions_grace_ends_at
  ON billing.subscriptions (grace_ends_at)
  WHERE grace_ends_at IS NOT NULL;

-- Idempotency ledger for recurring subscription credit grants.
-- Ensures we don't double-grant on webhook retries.
CREATE TABLE IF NOT EXISTS billing.subscription_credit_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL REFERENCES billing.subscriptions(id),
    credit_type_id UUID NOT NULL REFERENCES billing.credit_types(id),
    period_end TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    UNIQUE(subscription_id, credit_type_id, period_end)
);

CREATE INDEX IF NOT EXISTS idx_subscription_credit_grants_subscription ON billing.subscription_credit_grants(subscription_id);
CREATE INDEX IF NOT EXISTS idx_subscription_credit_grants_credit_type ON billing.subscription_credit_grants(credit_type_id);
CREATE INDEX IF NOT EXISTS idx_subscription_credit_grants_period_end ON billing.subscription_credit_grants(period_end);
