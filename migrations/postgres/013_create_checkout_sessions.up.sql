-- Migration 014: Checkout sessions

SET lock_timeout = '10s';
SET statement_timeout = '300s';

CREATE TABLE IF NOT EXISTS billing.checkout_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    price_id UUID NOT NULL REFERENCES billing.prices(id),
    mode TEXT NOT NULL CHECK (mode IN ('one_off', 'subscription')),
    processor TEXT NOT NULL,
    status TEXT NOT NULL,
    amount BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'usd',
    expires_at TIMESTAMPTZ,
    reference TEXT,
    transaction_id TEXT,
    payment_id UUID REFERENCES billing.payments(id),
    subscription_id UUID REFERENCES billing.subscriptions(id),
    processor_fields JSONB,
    processor_state JSONB,
    metadata JSONB,
    idempotency_key TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE UNIQUE INDEX IF NOT EXISTS checkout_sessions_processor_transaction_id_idx
    ON billing.checkout_sessions (processor, transaction_id)
    WHERE transaction_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS checkout_sessions_processor_reference_idx
    ON billing.checkout_sessions (processor, reference)
    WHERE reference IS NOT NULL;

CREATE INDEX IF NOT EXISTS checkout_sessions_user_status_idx
    ON billing.checkout_sessions (user_id, status);

CREATE INDEX IF NOT EXISTS checkout_sessions_expires_at_idx
    ON billing.checkout_sessions (expires_at);
