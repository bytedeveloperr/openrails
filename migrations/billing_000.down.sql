-- Consolidated Billing, Subscriptions, and Payments Schema Rollback
-- This file reverses all billing-related changes in the proper dependency order
-- Drops: subscription_plans, subscription_events, subscriptions, products, prices, 
--        user_role_grants, payment_methods, payments, and supporting tables

-- Set timeouts to prevent hanging migrations
SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- ============================================================================
-- SECTION 1: DROP FOREIGN KEY CONSTRAINTS AND REFERENCES
-- ============================================================================

-- 1.1: Remove foreign key constraints from subscriptions table
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS fk_subscriptions_plan_id;
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS fk_subscriptions_price_id;
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS fk_subscriptions_payment_method_id;

-- 1.2: Remove foreign key constraints from user_role_grants
ALTER TABLE user_role_grants DROP CONSTRAINT IF EXISTS fk_user_role_grants_sub_source_id;
ALTER TABLE user_role_grants DROP CONSTRAINT IF EXISTS fk_user_role_grants_purchase_source_id;

-- 1.3: Remove indexes from subscriptions before dropping referenced tables
DROP INDEX IF EXISTS idx_subscriptions_user_active;
DROP INDEX IF EXISTS idx_subscriptions_payment_method_id;
DROP INDEX IF EXISTS idx_subscriptions_next_retry_at;
DROP INDEX IF EXISTS idx_subscriptions_processor_subscription_id;
DROP INDEX IF EXISTS idx_subscriptions_processor;
DROP INDEX IF EXISTS idx_subscriptions_status;
DROP INDEX IF EXISTS idx_subscriptions_price_id;
DROP INDEX IF EXISTS idx_subscriptions_plan_id;
DROP INDEX IF EXISTS idx_subscriptions_user_id;

-- ============================================================================
-- SECTION 2: DROP SUPPORTING TABLES AND RESTORE LEGACY STRUCTURES
-- ============================================================================

-- 2.1: Drop notification_queue table
DROP INDEX IF EXISTS idx_notification_queue_created_at;
DROP INDEX IF EXISTS idx_notification_queue_is_read;
DROP INDEX IF EXISTS idx_notification_queue_type;
DROP INDEX IF EXISTS idx_notification_queue_user_id;
DROP TABLE IF EXISTS notification_queue;

-- 2.2: Drop user_role_grant_extensions table and enum
DROP INDEX IF EXISTS idx_urge_subscription_id;
DROP INDEX IF EXISTS idx_urge_created_at;
DROP INDEX IF EXISTS idx_urge_kind;
DROP INDEX IF EXISTS idx_urge_grant_id;
DROP TABLE IF EXISTS user_role_grant_extensions;
DROP TYPE IF EXISTS extension_kind CASCADE;

-- 2.3: Restore original purchases table structure (rename from payments)
DO $$
BEGIN
    -- Rename payments back to purchases if payments exists
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'payments') THEN
        -- First add back the updated_at column that was dropped
        ALTER TABLE payments ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp;
        
        -- Remove columns that were added during consolidation
        ALTER TABLE payments DROP COLUMN IF EXISTS user_role_grant_id;
        ALTER TABLE payments DROP COLUMN IF EXISTS extension_days;
        ALTER TABLE payments DROP COLUMN IF EXISTS subscription_id;
        
        -- Rename the table back
        ALTER TABLE payments RENAME TO purchases;
        
        -- Rename associated indexes
        IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_payments_subscription_id') THEN
            DROP INDEX IF EXISTS idx_payments_subscription_id;
        END IF;
        
        IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_payments_user_id') THEN
            ALTER INDEX idx_payments_user_id RENAME TO idx_purchases_user_id;
        END IF;
        
        IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_payments_price_id') THEN
            ALTER INDEX idx_payments_price_id RENAME TO idx_purchases_price_id;
        END IF;
        
        IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_payments_processor') THEN
            ALTER INDEX idx_payments_processor RENAME TO idx_purchases_processor;
        END IF;
        
        IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_payments_status') THEN
            ALTER INDEX idx_payments_status RENAME TO idx_purchases_status;
        END IF;
        
        IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_payments_purchased_at') THEN
            ALTER INDEX idx_payments_purchased_at RENAME TO idx_purchases_purchased_at;
        END IF;
    END IF;
END$$;

-- ============================================================================
-- SECTION 3: RESTORE LEGACY PAYMENT METHOD STRUCTURES
-- ============================================================================

-- 3.1: Recreate mobius_payment_methods table and migrate data back
CREATE TABLE IF NOT EXISTS mobius_payment_methods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    vault_id TEXT NOT NULL, -- Mobius vault ID
    billing_id TEXT, -- Mobius billing ID (nullable)
    initial_tx_id TEXT, -- First successful transaction
    status BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    
    UNIQUE(user_id, vault_id) -- Prevent duplicate vault entries per user
);

CREATE INDEX IF NOT EXISTS idx_mobius_payment_methods_user_id ON mobius_payment_methods(user_id);
CREATE INDEX IF NOT EXISTS idx_mobius_payment_methods_vault_id ON mobius_payment_methods(vault_id);
CREATE INDEX IF NOT EXISTS idx_mobius_payment_methods_billing_id ON mobius_payment_methods(billing_id) WHERE billing_id IS NOT NULL;

-- Migrate data back from payment_methods to mobius_payment_methods
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'payment_methods') THEN
        INSERT INTO mobius_payment_methods (
            id,
            user_id,
            vault_id,
            billing_id,
            initial_tx_id,
            status,
            created_at,
            updated_at
        )
        SELECT 
            id,
            user_id,
            vault_id,
            billing_id,
            initial_transaction_id,
            is_active,
            created_at,
            updated_at
        FROM payment_methods
        WHERE processor = 'mobius'
        ON CONFLICT (user_id, vault_id) DO NOTHING;
    END IF;
END$$;

-- 3.2: Drop payment_methods table and indexes
DROP INDEX IF EXISTS idx_payment_methods_is_active;
DROP INDEX IF EXISTS idx_payment_methods_processor_vault_id;
DROP INDEX IF EXISTS idx_payment_methods_vault_id;
DROP INDEX IF EXISTS idx_payment_methods_processor;
DROP INDEX IF EXISTS idx_payment_methods_user_id;
DROP TABLE IF EXISTS payment_methods;

-- ============================================================================
-- SECTION 4: RESTORE LEGACY SUBSCRIPTION STRUCTURES
-- ============================================================================

-- 4.1: Recreate subscription_logs table and migrate data back from subscription_events
CREATE TABLE IF NOT EXISTS subscription_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id TEXT NOT NULL,
    change_type TEXT NOT NULL,
    old_value TEXT,
    new_value TEXT,
    metadata TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    created_by TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

-- Migrate data back from subscription_events to subscription_logs
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'subscription_events') THEN
        INSERT INTO subscription_logs (subscription_id, change_type, old_value, new_value, metadata, created_at, created_by)
        SELECT 
            se.subscription_id::TEXT,
            CASE 
                WHEN se.event_type = 'status_change' THEN 'status_update'
                WHEN se.event_type = 'extension' THEN 'extension'
                ELSE 'updated'
            END,
            se.metadata->>'old_value',
            se.metadata->>'new_value',
            se.metadata->>'metadata',
            se.created_at,
            se.created_by
        FROM subscription_events se
        WHERE se.metadata IS NOT NULL
            AND se.metadata->>'old_value' IS NOT NULL
        ON CONFLICT DO NOTHING;
    END IF;
END$$;

-- ============================================================================
-- SECTION 5: REVERT SUBSCRIPTIONS TABLE TO LEGACY STRUCTURE
-- ============================================================================

-- 5.1: Add back legacy columns to subscriptions
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS description TEXT DEFAULT '';
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS membership TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS plan TEXT DEFAULT '';
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS total DECIMAL(10,2) DEFAULT 0;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS failure_reason TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS failed_at TIMESTAMPTZ;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS failure_code TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS processor_transaction_id TEXT;

-- Migrate failure data back from subscription_events
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'subscription_events') THEN
        UPDATE subscriptions 
        SET 
            failure_reason = se.failure_reason,
            failure_code = se.failure_code,
            failed_at = se.created_at
        FROM subscription_events se
        WHERE subscriptions.id = se.subscription_id 
            AND se.event_type = 'charge_failed'
            AND se.failure_reason IS NOT NULL;
    END IF;
END$$;

-- 5.2: Set default plan value from plan relationship
UPDATE subscriptions 
SET plan = COALESCE(sp.name, 'unknown')
FROM subscription_plans sp
WHERE subscriptions.plan_id = sp.id;

-- 5.3: Make legacy columns NOT NULL where required
ALTER TABLE subscriptions ALTER COLUMN description SET NOT NULL;
UPDATE subscriptions SET plan = 'unknown' WHERE plan IS NULL;
ALTER TABLE subscriptions ALTER COLUMN plan SET NOT NULL;
ALTER TABLE subscriptions ALTER COLUMN total SET NOT NULL;

-- 5.4: Change status back to text type
ALTER TABLE subscriptions ALTER COLUMN status TYPE TEXT;

-- 5.5: Remove new columns added during consolidation
ALTER TABLE subscriptions DROP COLUMN IF EXISTS plan_id;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS price_id;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS payment_method_id;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS last_retry_at;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS retry_attempts;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS next_retry_at;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS cancel_type;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS cancel_feedback;

-- 5.6: Make fields nullable again that were made NOT NULL during consolidation
ALTER TABLE subscriptions ALTER COLUMN started_at DROP NOT NULL;
ALTER TABLE subscriptions ALTER COLUMN processor DROP NOT NULL;
ALTER TABLE subscriptions ALTER COLUMN processor_subscription_id DROP NOT NULL;

-- ============================================================================
-- SECTION 6: DROP SUBSCRIPTION INFRASTRUCTURE TABLES
-- ============================================================================

-- 6.1: Drop subscription_events table
DROP INDEX IF EXISTS idx_subscription_events_subscription_type;
DROP INDEX IF EXISTS idx_subscription_events_created_at;
DROP INDEX IF EXISTS idx_subscription_events_event_type;
DROP INDEX IF EXISTS idx_subscription_events_subscription_id;
DROP TABLE IF EXISTS subscription_events;

-- 6.2: Drop user_role_grants table
DROP INDEX IF EXISTS idx_user_role_grants_expires;
DROP INDEX IF EXISTS idx_user_role_grants_role_id;
DROP INDEX IF EXISTS idx_user_role_grants_user_id;
DROP TABLE IF EXISTS user_role_grants;

-- ============================================================================
-- SECTION 7: DROP PRODUCT AND PRICING TABLES
-- ============================================================================

-- 7.1: Drop prices table
ALTER TABLE prices DROP CONSTRAINT IF EXISTS unique_prices_product_amount_cycle;
DROP INDEX IF EXISTS idx_prices_is_active;
DROP INDEX IF EXISTS idx_prices_ccbill_price_id;
DROP INDEX IF EXISTS idx_prices_mobius_plan_id;
DROP INDEX IF EXISTS idx_prices_product_id;
DROP TABLE IF EXISTS prices;

-- 7.2: Drop products table
DROP INDEX IF EXISTS idx_products_is_active;
DROP INDEX IF EXISTS idx_products_role_id;
DROP INDEX IF EXISTS idx_products_slug;
DROP TABLE IF EXISTS products;

-- ============================================================================
-- SECTION 8: DROP SUBSCRIPTION PLANS TABLE
-- ============================================================================

-- 8.1: Drop subscription_plans table
DROP INDEX IF EXISTS idx_subscription_plans_name;
DROP INDEX IF EXISTS idx_subscription_plans_is_active;
DROP TABLE IF EXISTS subscription_plans;

-- ============================================================================
-- SECTION 9: DROP ENUMS AND TYPES
-- ============================================================================

-- 9.1: Drop all custom types in reverse dependency order
DROP TYPE IF EXISTS subscription_status CASCADE;
DROP TYPE IF EXISTS purchase_status CASCADE;
DROP TYPE IF EXISTS processor_type CASCADE;

-- ============================================================================
-- SECTION 10: RESTORE LEGACY STRUCTURES
-- ============================================================================

-- 10.1: Restore processor_subscriptions table if it was part of the original schema
CREATE TABLE IF NOT EXISTS processor_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    processor_subscription_id TEXT NOT NULL,
    processor_transaction_id TEXT NOT NULL,
    processor TEXT NOT NULL,
    subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    deleted_at TIMESTAMPTZ NULL
);

-- Recreate original subscriptions table indexes
CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_processor ON subscriptions(processor);