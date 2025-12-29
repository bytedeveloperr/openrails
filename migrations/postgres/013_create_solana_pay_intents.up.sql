-- Migration 013: Solana Pay Transaction Request intents

SET lock_timeout = '10s';
SET statement_timeout = '300s';

CREATE TABLE IF NOT EXISTS billing.solana_pay_intents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    recipient TEXT NOT NULL,
    token_mint TEXT,
    amount BIGINT NOT NULL,
    reference TEXT NOT NULL UNIQUE,
    message TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);
