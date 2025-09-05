-- Set timeouts to prevent hanging migrations
SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- ============================================================================
-- SECTION 1: CORE SUBSCRIPTION TABLES
-- ============================================================================

-- 1.1: Create subscription_plans table
CREATE TABLE IF NOT EXISTS subscription_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT,
    price_usd DECIMAL(10,2) NOT NULL,
    billing_cycle INTEGER NOT NULL,
    features TEXT[],
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_subscription_plans_is_active ON subscription_plans(is_active);
CREATE INDEX IF NOT EXISTS idx_subscription_plans_name ON subscription_plans(name);

COMMENT ON TABLE subscription_plans IS 'Defines available subscription plans with pricing and features';

-- 1.2: Create subscription status enum
DROP TYPE IF EXISTS subscription_status CASCADE;
CREATE TYPE subscription_status AS ENUM ('pending', 'active', 'expired', 'cancelled', 'failed', 'past_due');

-- 1.3: Create subscriptions table
CREATE TABLE IF NOT EXISTS subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL, -- Zitadel subject (sub)
    plan_id UUID NOT NULL REFERENCES subscription_plans(id),
    price_id UUID, -- References prices table (created later)
    status subscription_status NOT NULL DEFAULT 'pending',
    
    -- Processor information
    processor TEXT NOT NULL DEFAULT 'ccbill',
    processor_subscription_id TEXT NOT NULL DEFAULT '',
    payment_method_id UUID, -- References payment_methods table (created later)
    
    -- Billing period tracking
    current_period_starts_at TIMESTAMPTZ,
    current_period_ends_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    ended_at TIMESTAMPTZ,
    
    -- Retry fields for manual rebilling
    last_retry_at TIMESTAMPTZ,
    retry_attempts INTEGER DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    
    -- Cancellation tracking
    cancelled_at TIMESTAMPTZ,
    cancel_type TEXT, -- 'user', 'admin', 'failed_payment', 'expired'
    cancel_feedback TEXT,
    
    -- Financial information
    currency TEXT NOT NULL DEFAULT 'USD',
    amount DECIMAL(10,2) NOT NULL DEFAULT 0,
    billing_cycle INTEGER NOT NULL DEFAULT 30,
    
    -- Metadata
    gateway_response JSONB,
    public_id INTEGER,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

-- Create indexes for subscriptions
CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_plan_id ON subscriptions(plan_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_price_id ON subscriptions(price_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_processor ON subscriptions(processor);
CREATE INDEX IF NOT EXISTS idx_subscriptions_processor_subscription_id ON subscriptions(processor_subscription_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_next_retry_at ON subscriptions(next_retry_at) WHERE next_retry_at IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_user_active ON subscriptions(user_id) WHERE status = 'active';

COMMENT ON INDEX idx_subscriptions_user_active IS 'Ensures each user can have only one active subscription at a time';

-- 1.4: Create subscription_events table for audit trail
CREATE TABLE IF NOT EXISTS subscription_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    status TEXT,
    amount DECIMAL(10,2),
    currency TEXT,
    failure_reason TEXT,
    failure_code TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    created_by TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_subscription_events_subscription_id ON subscription_events(subscription_id);
CREATE INDEX IF NOT EXISTS idx_subscription_events_event_type ON subscription_events(event_type);
CREATE INDEX IF NOT EXISTS idx_subscription_events_created_at ON subscription_events(created_at);
CREATE INDEX IF NOT EXISTS idx_subscription_events_subscription_type ON subscription_events(subscription_id, event_type);

COMMENT ON TABLE subscription_events IS 'Audit trail for all subscription-related events and changes';
COMMENT ON COLUMN subscription_events.metadata IS 'JSONB field for storing event-specific data';

-- ============================================================================
-- SECTION 2: PRODUCTS AND PRICING
-- ============================================================================

-- 2.1: Create products table
CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT,
    role_slug TEXT NOT NULL, -- Role identifier - no FK dependency on external roles table
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_products_slug ON products(slug);
CREATE INDEX IF NOT EXISTS idx_products_role_slug ON products(role_slug);
CREATE INDEX IF NOT EXISTS idx_products_is_active ON products(is_active);

-- 2.2: Create prices table
CREATE TABLE IF NOT EXISTS prices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    amount DECIMAL(10,2) NOT NULL,
    currency TEXT NOT NULL,
    billing_cycle_days INTEGER, -- 30 for monthly, 365 for yearly, NULL for one-time
    mobius_plan_id TEXT, -- Mobius processor plan ID
    ccbill_price_id TEXT, -- CCBill processor price ID  
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_prices_product_id ON prices(product_id);
CREATE INDEX IF NOT EXISTS idx_prices_mobius_plan_id ON prices(mobius_plan_id) WHERE mobius_plan_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_prices_ccbill_price_id ON prices(ccbill_price_id) WHERE ccbill_price_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_prices_is_active ON prices(is_active);

ALTER TABLE prices DROP CONSTRAINT IF EXISTS unique_prices_product_amount_cycle;
ALTER TABLE prices ADD CONSTRAINT unique_prices_product_amount_cycle 
    UNIQUE (product_id, amount, currency, billing_cycle_days);

-- Add foreign key reference from subscriptions to prices
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'fk_subscriptions_price_id'
    ) THEN
        ALTER TABLE subscriptions 
        ADD CONSTRAINT fk_subscriptions_price_id 
        FOREIGN KEY (price_id) REFERENCES prices(id);
    END IF;
END$$;

-- ============================================================================
-- SECTION 3: USER ROLE GRANTS AND EXTENSIONS
-- ============================================================================

-- 3.1: Create user_role_grants table
CREATE TABLE IF NOT EXISTS user_role_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL, -- Zitadel subject (sub)
    role_slug TEXT NOT NULL, -- Role identifier - no FK dependency on external roles table
    granted_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    auto_expires_at TIMESTAMPTZ, -- NULL means never expires
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_user_role_grants_user_id ON user_role_grants(user_id);
CREATE INDEX IF NOT EXISTS idx_user_role_grants_role_slug ON user_role_grants(role_slug);
CREATE INDEX IF NOT EXISTS idx_user_role_grants_expires ON user_role_grants(auto_expires_at) WHERE auto_expires_at IS NOT NULL;

-- 3.2: Create extension_kind enum
DROP TYPE IF EXISTS extension_kind CASCADE;
CREATE TYPE extension_kind AS ENUM ('admin', 'grace');

-- 3.3: Create user_role_grant_extensions table
CREATE TABLE IF NOT EXISTS user_role_grant_extensions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_role_grant_id UUID NOT NULL REFERENCES user_role_grants(id) ON DELETE CASCADE,
    kind extension_kind NOT NULL,
    extension_days INTEGER NOT NULL DEFAULT 0,
    subscription_id UUID REFERENCES subscriptions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_urge_grant_id ON user_role_grant_extensions(user_role_grant_id);
CREATE INDEX IF NOT EXISTS idx_urge_kind ON user_role_grant_extensions(kind);
CREATE INDEX IF NOT EXISTS idx_urge_created_at ON user_role_grant_extensions(created_at);
CREATE INDEX IF NOT EXISTS idx_urge_subscription_id ON user_role_grant_extensions(subscription_id);

COMMENT ON TABLE user_role_grant_extensions IS 'Tracks admin and grace extensions to role grants (non-payment adjustments)';

-- ============================================================================
-- SECTION 4: PAYMENT PROCESSING
-- ============================================================================

-- 4.1: Create payment_methods table
CREATE TABLE IF NOT EXISTS payment_methods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL, -- Zitadel subject (sub)
    processor VARCHAR(50) NOT NULL, -- 'mobius', 'ccbill', etc.
    
    -- Processor-specific vault/payment method identifiers
    vault_id VARCHAR(255) NOT NULL, -- Primary identifier in processor's system
    billing_id VARCHAR(255), -- Secondary identifier (e.g., subscription ID)
    initial_transaction_id VARCHAR(255) NOT NULL, -- Transaction that created this vault
    
    -- Payment method status and metadata
    is_active BOOLEAN NOT NULL DEFAULT true, -- Can this method be used for rebills?
    last_four VARCHAR(4), -- Last 4 digits of card
    card_type VARCHAR(50), -- 'Visa', 'MasterCard', etc.
    expiry_date VARCHAR(5), -- 'MM/YY' format
    failure_reason TEXT, -- Reason if inactive
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_payment_methods_user_id ON payment_methods(user_id);
CREATE INDEX IF NOT EXISTS idx_payment_methods_processor ON payment_methods(processor);
CREATE INDEX IF NOT EXISTS idx_payment_methods_vault_id ON payment_methods(vault_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_methods_processor_vault_id ON payment_methods(processor, vault_id);
CREATE INDEX IF NOT EXISTS idx_payment_methods_is_active ON payment_methods(is_active) WHERE is_active = true;

COMMENT ON TABLE payment_methods IS 'Generalized payment method table supporting multiple processors.';
COMMENT ON COLUMN payment_methods.processor IS 'Payment processor type: mobius, ccbill, stripe, etc.';
COMMENT ON COLUMN payment_methods.vault_id IS 'Primary payment method identifier in the processor system';

-- Add payment_method_id reference to subscriptions table
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'fk_subscriptions_payment_method_id'
    ) THEN
        ALTER TABLE subscriptions 
        ADD CONSTRAINT fk_subscriptions_payment_method_id 
        FOREIGN KEY (payment_method_id) REFERENCES payment_methods(id) ON DELETE SET NULL;
    END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_subscriptions_payment_method_id ON subscriptions(payment_method_id);

-- 4.2: Create processor and purchase status enums
DROP TYPE IF EXISTS processor_type CASCADE;
CREATE TYPE processor_type AS ENUM ('paypal', 'solana', 'stripe', 'crypto', 'mobius', 'ccbill');

DROP TYPE IF EXISTS purchase_status CASCADE;
CREATE TYPE purchase_status AS ENUM ('pending', 'completed', 'failed', 'refunded');

-- 4.3: Create payments table (formerly purchases)
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL, -- Zitadel subject (sub)
    price_id UUID NOT NULL REFERENCES prices(id),
    processor processor_type NOT NULL,
    processor_transaction_id TEXT NOT NULL,
    amount_usd DECIMAL(10,2) NOT NULL,
    status purchase_status NOT NULL DEFAULT 'pending',
    
    -- Role grant tracking
    user_role_grant_id UUID REFERENCES user_role_grants(id) ON DELETE SET NULL,
    extension_days INTEGER,
    
    -- Subscription linkage
    subscription_id UUID REFERENCES subscriptions(id) ON DELETE SET NULL,
    
    -- Metadata
    metadata JSONB DEFAULT '{}',
    purchased_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    
    UNIQUE(processor, processor_transaction_id) -- Prevent duplicate transactions
);

CREATE INDEX IF NOT EXISTS idx_payments_user_id ON payments(user_id);
CREATE INDEX IF NOT EXISTS idx_payments_price_id ON payments(price_id);
CREATE INDEX IF NOT EXISTS idx_payments_processor ON payments(processor);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);
CREATE INDEX IF NOT EXISTS idx_payments_purchased_at ON payments(purchased_at);
CREATE INDEX IF NOT EXISTS idx_payments_subscription_id ON payments(subscription_id);

COMMENT ON COLUMN payments.subscription_id IS 'Links a payment to the subscription that generated it (nullable for one-off payments)';

-- 4.4: Create solana_transactions table (pending and confirmed Solana payments)
CREATE TABLE IF NOT EXISTS solana_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT, -- Zitadel subject (sub), nullable for anonymous intents
    signature TEXT, -- Solana transaction signature (set when confirmed)
    status TEXT NOT NULL, -- pending, confirmed, failed

    -- Payment details
    amount NUMERIC(18,9) NOT NULL,
    token TEXT NOT NULL, -- e.g., SOL, USDC
    token_mint TEXT NOT NULL, -- token mint address

    -- Addresses
    from_address TEXT NOT NULL,
    to_address TEXT NOT NULL,

    -- Optional references
    product_id UUID,
    purchase_id UUID,

    -- Blockchain metadata
    block_time TIMESTAMPTZ,
    slot BIGINT,
    confirmations INTEGER NOT NULL DEFAULT 0,
    transaction_fee NUMERIC(18,9),

    -- Processing metadata
    processing_result JSONB,
    error_message TEXT,

    -- QR/payment flow metadata
    qr_code_id TEXT,

    -- Expiration for pending transactions
    expires_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_solana_transactions_user_id ON solana_transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_solana_transactions_status ON solana_transactions(status);
CREATE INDEX IF NOT EXISTS idx_solana_transactions_expires_at ON solana_transactions(expires_at) WHERE expires_at IS NOT NULL;

-- ============================================================================
-- SECTION 5: SUPPORTING TABLES
-- ============================================================================

-- 5.1: Create notification_queue table
CREATE TABLE IF NOT EXISTS notification_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL, -- Zitadel subject (sub)
    notification_type TEXT NOT NULL, -- role_expired, subscription_failed, etc.
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    metadata JSONB DEFAULT '{}',
    is_read BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    read_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notification_queue_user_id ON notification_queue(user_id);
CREATE INDEX IF NOT EXISTS idx_notification_queue_type ON notification_queue(notification_type);
CREATE INDEX IF NOT EXISTS idx_notification_queue_is_read ON notification_queue(is_read);
CREATE INDEX IF NOT EXISTS idx_notification_queue_created_at ON notification_queue(created_at);

-- 5.2: Create solana_transactions table (tracks pending/confirmed Solana payments)
CREATE TABLE IF NOT EXISTS solana_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT, -- Zitadel subject (sub); nullable for anonymous/pending

    -- Transaction details
    signature TEXT, -- on-chain signature when confirmed
    status TEXT NOT NULL, -- pending, confirmed, failed

    -- Payment details
    amount NUMERIC(18,9) NOT NULL,
    token TEXT NOT NULL, -- SOL, USDC, PYUSD
    token_mint TEXT NOT NULL,

    -- Addresses
    from_address TEXT NOT NULL,
    to_address TEXT NOT NULL,

    -- Optional references
    product_id UUID,
    purchase_id UUID,

    -- Blockchain details
    block_time TIMESTAMPTZ,
    slot BIGINT,
    confirmations INTEGER NOT NULL DEFAULT 0,
    transaction_fee NUMERIC(18,9),

    -- Processing metadata
    processing_result JSONB,
    error_message TEXT,

    -- QR/payment UI reference
    qr_code_id TEXT,

    -- Pending expiration
    expires_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE INDEX IF NOT EXISTS idx_solana_tx_user_id ON solana_transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_solana_tx_status ON solana_transactions(status);
CREATE INDEX IF NOT EXISTS idx_solana_tx_created_at ON solana_transactions(created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_solana_tx_signature ON solana_transactions(signature) WHERE signature IS NOT NULL;

-- ============================================================================
-- SECTION 6: DATA MIGRATION FROM LEGACY TABLES
-- ============================================================================

-- 6.2: Migrate data from purchases table if it exists and hasn't been renamed yet
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'purchases') 
       AND NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'payments') THEN
        -- First rename the table
        ALTER TABLE purchases RENAME TO payments;
        
        -- Rename any associated indexes
        IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_purchases_subscription_id') THEN
            ALTER INDEX idx_purchases_subscription_id RENAME TO idx_payments_subscription_id;
        END IF;
    END IF;
END$$;

-- ============================================================================
-- SECTION 7: CLEANUP AND FINAL ADJUSTMENTS
-- ============================================================================


-- 7.3: Add comments for documentation
COMMENT ON TABLE subscriptions IS 'Core subscription records tracking user billing relationships';
COMMENT ON TABLE products IS 'Product definitions that can be purchased or subscribed to';
COMMENT ON TABLE prices IS 'Pricing tiers for products with processor-specific identifiers';
COMMENT ON TABLE user_role_grants IS 'Tracks which roles are granted to users and when they expire';
COMMENT ON TABLE payments IS 'Records of all payment transactions (formerly purchases table)';
COMMENT ON TABLE notification_queue IS 'Queue for user notifications related to billing and subscriptions';
