-- Add wallet_address field to payment_methods table for Solana wallets
-- This migration adds support for storing Solana wallet addresses as payment methods

-- Set timeouts to prevent hanging migrations
SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- Add wallet_address column to payment_methods table
ALTER TABLE payment_methods 
ADD COLUMN IF NOT EXISTS wallet_address TEXT;

-- Add index for wallet_address lookups
CREATE INDEX IF NOT EXISTS idx_payment_methods_wallet_address 
ON payment_methods(wallet_address) 
WHERE wallet_address IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN payment_methods.wallet_address IS 'Solana wallet address for crypto payment methods (Base58 encoded)';