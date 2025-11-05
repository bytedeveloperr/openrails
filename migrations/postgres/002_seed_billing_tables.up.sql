-- 00002_seed_billings_tables.sql
-- Seed data for billing tables - products, prices, and default subscription plans

-- Explicitly set schema to ensure all objects are created in the correct place
-- Set timeouts to prevent hanging migrations
SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- ============================================================================
-- SECTION 0: CLEANUP LEGACY DATA
-- ============================================================================

-- Update any NULL processor_subscription_id values with defaults
UPDATE subscriptions 
SET processor_subscription_id = 'LEGACY-' || id::text
WHERE processor_subscription_id IS NULL OR processor_subscription_id = '';

-- Set default started_at for subscriptions that don't have it
UPDATE subscriptions 
SET started_at = created_at 
WHERE started_at IS NULL;

-- ============================================================================
-- SECTION 2: SEED PRODUCTS CATALOG
-- ============================================================================

-- Insert product catalog
INSERT INTO products (slug, display_name, description, entitlements_spec) VALUES
-- Basic tier
('basic_membership', 'Basic Membership', 'Essential access to the standard catalog and community features.', '{"basic": null}'::jsonb),

-- Premium tier
('premium_membership', 'Premium Membership', 'Everything in Basic plus premium catalog access and enhanced perks.', '{"basic": null, "premium": null}'::jsonb)

ON CONFLICT (slug) DO UPDATE SET 
    display_name = EXCLUDED.display_name,
    description = EXCLUDED.description,
    entitlements_spec = EXCLUDED.entitlements_spec,
    updated_at = current_timestamp;

-- ============================================================================
-- SECTION 3: SEED PRICING TIERS
-- ============================================================================

-- Insert pricing tiers
DO $$
DECLARE
    basic_id UUID;
    premium_id UUID;
BEGIN
    -- Get product IDs
    SELECT id INTO basic_id FROM products WHERE slug = 'basic_membership';
    SELECT id INTO premium_id FROM products WHERE slug = 'premium_membership';

    -- Basic tier pricing
    INSERT INTO prices (product_id, display_name, amount, currency, billing_cycle_days, ccbill_price_id, nmi_plan_id, nmi_provider, is_active) VALUES
    (basic_id, 'Basic Monthly', 4.99, 'USD', 30, '75383d6a-41d4-4bd0-ac12-6c8c37fde5e5', 'basic_monthly', 'mobius', true)
    ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO NOTHING;

    -- Premium tier pricing
    INSERT INTO prices (product_id, display_name, amount, currency, billing_cycle_days, ccbill_price_id, nmi_plan_id, nmi_provider, is_active) VALUES
    (premium_id, 'Premium Monthly', 9.99, 'USD', 30, '75383d6a-41d4-4bd0-ac12-6c8c37fde5e5', 'premium_monthly', 'mobius', true)
    ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO NOTHING;

END$$;

-- Add helpful comments for operators
COMMENT ON TABLE products IS 'Product catalog - modify these via your application, not directly in billing service';
COMMENT ON TABLE prices IS 'Pricing tiers with processor integration - modify these via your application for new campaigns';
COMMENT ON COLUMN prices.is_active IS 'Set to false to disable pricing tier without deleting (useful for campaigns)';
COMMENT ON COLUMN prices.ccbill_price_id IS 'CCBill FlexForm price identifier - update when creating new CCBill products';
COMMENT ON COLUMN prices.nmi_plan_id IS 'NMI plan identifier - update when creating new NMI plans';
COMMENT ON COLUMN prices.nmi_provider IS 'NMI provider slug (e.g., mobius) for multi-tenant gateways';
