-- Create table to persist Solana wallet verification challenges
CREATE TABLE IF NOT EXISTS solana_wallet_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    address TEXT NOT NULL,
    message TEXT NOT NULL,
    nonce TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    UNIQUE(user_id, address)
);

CREATE INDEX IF NOT EXISTS idx_solana_wallet_challenges_user ON solana_wallet_challenges(user_id);
CREATE INDEX IF NOT EXISTS idx_solana_wallet_challenges_expires ON solana_wallet_challenges(expires_at);
