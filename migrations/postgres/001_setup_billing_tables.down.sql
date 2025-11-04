-- Explicitly set schema to ensure all objects are created in the correct place
SET search_path = billing, public;
-- Set timeouts to prevent hanging migrations
SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- Drop in reverse dependency order
DROP TABLE IF EXISTS solana_wallet_challenges CASCADE;
DROP TABLE IF EXISTS solana_wallets CASCADE;
DROP TABLE IF EXISTS solana_transactions CASCADE;
DROP TABLE IF EXISTS solana_payment_intents CASCADE;
DROP TABLE IF EXISTS notification_queue CASCADE;
DROP TABLE IF EXISTS payments CASCADE;
DROP TABLE IF EXISTS payment_methods CASCADE;
DROP TABLE IF EXISTS entitlements CASCADE;
DROP TABLE IF EXISTS prices CASCADE;
DROP TABLE IF EXISTS products CASCADE;
DROP TABLE IF EXISTS subscriptions CASCADE;

-- Drop enums created by this migration
DROP TYPE IF EXISTS purchase_status;
DROP TYPE IF EXISTS processor_type;
DROP TYPE IF EXISTS subscription_status;
