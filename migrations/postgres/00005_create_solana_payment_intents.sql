-- Create solana_payment_intents table to unify Solana payment flows
CREATE TABLE IF NOT EXISTS solana_payment_intents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    price_id UUID NOT NULL,
    flow_type TEXT NOT NULL, -- direct | solanapay
    token TEXT NOT NULL,
    token_mint TEXT NOT NULL,
    amount DECIMAL(18,9) NOT NULL,
    currency TEXT NOT NULL,
    expected_amount_lamports BIGINT NOT NULL,
    payer_wallet TEXT,
    recipient_wallet TEXT NOT NULL,
    reference TEXT,
    memo TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    signature TEXT,
    transaction_signature TEXT,
    error_message TEXT,
    expires_at TIMESTAMPTZ,
    confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    UNIQUE(reference)
);

CREATE INDEX IF NOT EXISTS idx_solana_payment_intents_user_status ON solana_payment_intents(user_id, status);
CREATE INDEX IF NOT EXISTS idx_solana_payment_intents_reference ON solana_payment_intents(reference) WHERE reference IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_solana_payment_intents_expires ON solana_payment_intents(expires_at) WHERE expires_at IS NOT NULL;

-- Link solana_transactions to payment intents
ALTER TABLE solana_transactions
    ADD COLUMN IF NOT EXISTS intent_id UUID REFERENCES solana_payment_intents(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_solana_transactions_intent_id ON solana_transactions(intent_id);
