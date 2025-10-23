-- bun:up
-- 00002_seed_billings_tables.sql
-- Seed data for billing tables - products, prices, and default subscription plans

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
('basic_membership', 'Basic Membership', 'Access to standard content library and community features', '{"basic": null}'::jsonb),

-- Premium tier  
('premium_membership', 'Premium Membership', 'Full access to exclusive content, HD streaming, and priority support', '{"premium": null}'::jsonb),

-- Creator tier
('creator_membership', 'Creator Membership', 'Everything in Premium plus creator tools, analytics, and revenue sharing', '{"creator": null, "premium": null}'::jsonb),

-- Enterprise tier
('enterprise_membership', 'Enterprise Membership', 'Custom solutions for teams with API access, SSO, and dedicated support', '{"enterprise": null, "premium": null}'::jsonb)

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
    creator_id UUID;
    enterprise_id UUID;
BEGIN
    -- Get product IDs
    SELECT id INTO basic_id FROM products WHERE slug = 'basic_membership';
    SELECT id INTO premium_id FROM products WHERE slug = 'premium_membership';
    SELECT id INTO creator_id FROM products WHERE slug = 'creator_membership';
    SELECT id INTO enterprise_id FROM products WHERE slug = 'enterprise_membership';

    -- Basic tier pricing
    INSERT INTO prices (product_id, display_name, amount, currency, billing_cycle_days, ccbill_price_id, nmi_plan_id, nmi_provider, is_active) VALUES
    (basic_id, 'Basic Monthly', 9.99, 'USD', 30, 'ccbill_basic_monthly', 'nmi_basic_monthly', 'mobius', true),
    (basic_id, 'Basic Annual', 99.99, 'USD', 365, 'ccbill_basic_annual', 'nmi_basic_annual', 'mobius', true)
    ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO NOTHING;

    -- Premium tier pricing
    INSERT INTO prices (product_id, display_name, amount, currency, billing_cycle_days, ccbill_price_id, nmi_plan_id, nmi_provider, is_active) VALUES
    (premium_id, 'Premium Monthly', 19.99, 'USD', 30, 'ccbill_premium_monthly', 'nmi_premium_monthly', 'mobius', true),
    (premium_id, 'Premium Annual', 199.99, 'USD', 365, 'ccbill_premium_annual', 'nmi_premium_annual', 'mobius', true),
    (premium_id, 'Premium Lifetime', 499.99, 'USD', NULL, 'ccbill_premium_lifetime', 'nmi_premium_lifetime', 'mobius', true)
    ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO NOTHING;

    -- Creator tier pricing
    INSERT INTO prices (product_id, display_name, amount, currency, billing_cycle_days, ccbill_price_id, nmi_plan_id, nmi_provider, is_active) VALUES
    (creator_id, 'Creator Monthly', 39.99, 'USD', 30, 'ccbill_creator_monthly', 'nmi_creator_monthly', 'mobius', true),
    (creator_id, 'Creator Annual', 399.99, 'USD', 365, 'ccbill_creator_annual', 'nmi_creator_annual', 'mobius', true),
    (creator_id, 'Creator Lifetime', 999.99, 'USD', NULL, 'ccbill_creator_lifetime', 'nmi_creator_lifetime', 'mobius', true)
    ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO NOTHING;

    -- Enterprise tier pricing
    INSERT INTO prices (product_id, display_name, amount, currency, billing_cycle_days, ccbill_price_id, nmi_plan_id, nmi_provider, is_active) VALUES
    (enterprise_id, 'Enterprise Monthly', 99.99, 'USD', 30, 'ccbill_enterprise_monthly', 'nmi_enterprise_monthly', 'mobius', true),
    (enterprise_id, 'Enterprise Annual', 999.99, 'USD', 365, 'ccbill_enterprise_annual', 'nmi_enterprise_annual', 'mobius', true)
    ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO NOTHING;

    -- ============================================================================
    -- SECTION 4: PROMOTIONAL PRICING (Disabled by Default)
    -- ============================================================================

    -- Special promotional pricing (disabled by default, can be activated for campaigns)
    INSERT INTO prices (product_id, display_name, amount, currency, billing_cycle_days, ccbill_price_id, nmi_plan_id, nmi_provider, is_active) VALUES
    (premium_id, 'Premium Monthly - Black Friday', 9.99, 'USD', 30, 'ccbill_premium_bf', 'nmi_premium_bf', 'mobius', false),
    (premium_id, 'Premium Monthly - New Year', 14.99, 'USD', 30, 'ccbill_premium_ny', 'nmi_premium_ny', 'mobius', false),
    (premium_id, 'Premium Trial - 7 Days', 0.99, 'USD', 7, 'ccbill_premium_trial', 'nmi_premium_trial', 'mobius', false),
    (basic_id, 'Basic Trial - 14 Days', 0.99, 'USD', 14, 'ccbill_basic_trial', 'nmi_basic_trial', 'mobius', false),
    (creator_id, 'Creator Trial - 30 Days', 9.99, 'USD', 30, 'ccbill_creator_trial', 'nmi_creator_trial', 'mobius', false)
    ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO NOTHING;

    -- ============================================================================
    -- SECTION 5: ALTERNATIVE CURRENCIES (Future Expansion)
    -- ============================================================================

    -- EUR pricing (disabled by default, enable when ready)
    INSERT INTO prices (product_id, display_name, amount, currency, billing_cycle_days, ccbill_price_id, nmi_plan_id, nmi_provider, is_active) VALUES
    (premium_id, 'Premium Monthly - EUR', 18.99, 'EUR', 30, 'ccbill_premium_monthly_eur', 'nmi_premium_monthly_eur', 'mobius', false),
    (premium_id, 'Premium Annual - EUR', 189.99, 'EUR', 365, 'ccbill_premium_annual_eur', 'nmi_premium_annual_eur', 'mobius', false)
    ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO NOTHING;

    -- GBP pricing (disabled by default)
    INSERT INTO prices (product_id, display_name, amount, currency, billing_cycle_days, ccbill_price_id, nmi_plan_id, nmi_provider, is_active) VALUES
    (premium_id, 'Premium Monthly - GBP', 16.99, 'GBP', 30, 'ccbill_premium_monthly_gbp', 'nmi_premium_monthly_gbp', 'mobius', false),
    (premium_id, 'Premium Annual - GBP', 169.99, 'GBP', 365, 'ccbill_premium_annual_gbp', 'nmi_premium_annual_gbp', 'mobius', false)
    ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO NOTHING;

END$$;

-- Add helpful comments
COMMENT ON TABLE products IS 'Product catalog - modify these via your application, not directly in billing service';
COMMENT ON TABLE prices IS 'Pricing tiers with processor integration - modify these via your application for new campaigns';
COMMENT ON COLUMN prices.is_active IS 'Set to false to disable pricing tier without deleting (useful for campaigns)';
COMMENT ON COLUMN prices.ccbill_price_id IS 'CCBill FlexForm price identifier - update when creating new CCBill products';
COMMENT ON COLUMN prices.nmi_plan_id IS 'NMI plan identifier - update when creating new NMI plans';
COMMENT ON COLUMN prices.nmi_provider IS 'NMI provider slug (e.g., mobius) for multi-tenant gateways';
