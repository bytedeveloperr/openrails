SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- Recreate subscription_credit_grants table (best-effort) for rollback.
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

