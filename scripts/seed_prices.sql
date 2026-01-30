
-- Seed a minimal Mobius/NMI and CCBill catalog for local E2E in psql shell.
--
-- Usage:
--   \i scripts/seed_prices.sql

\set ON_ERROR_STOP on

DO $$
BEGIN
  IF current_setting('server_version_num')::int < 120000 THEN
    RAISE EXCEPTION 'postgres 12+ required';
  END IF;
END $$;

INSERT INTO billing.products (slug, display_name, description, tier_group, tier_rank, is_active)
VALUES ('premium_monthly', 'E2E Mobius Plan', 'Local E2E product for Mobius/NMI/CCBill sandbox', 'e2e', 1, true)
ON CONFLICT (slug) DO UPDATE
SET display_name = EXCLUDED.display_name,
    description = EXCLUDED.description,
    tier_group = EXCLUDED.tier_group,
    tier_rank = EXCLUDED.tier_rank,
    is_active = true,
    updated_at = current_timestamp;

WITH p AS (
  SELECT id AS product_id FROM billing.products WHERE slug = 'premium_monthly'
)
INSERT INTO billing.prices (product_id, display_name, amount, currency, billing_cycle_days, processors, is_active)
SELECT
  p.product_id,
  'E2E Mobius Monthly (1 day cadence recommended for rebill tests)',
  999,
  'usd',
  1,
  jsonb_build_object(
    'mobius', jsonb_build_object('plan_id', 'premium_monthly', 'provider', 'mobius'),
    'ccbill', jsonb_build_object('plan_id', 'premium-monthly', 'provider', 'ccbill')
  ),
  true
FROM p
ON CONFLICT (product_id, amount, currency, billing_cycle_days) DO UPDATE
SET processors = EXCLUDED.processors,
    is_active = true,
    updated_at = current_timestamp;

SELECT 'product_id' AS key, id::text AS value FROM billing.products WHERE slug='premium_monthly'
UNION ALL
SELECT 'price_id' AS key, id::text AS value
FROM billing.prices
WHERE product_id = (SELECT id FROM billing.products WHERE slug='premium_monthly')
  AND amount = 999 AND currency='usd' AND billing_cycle_days=1;

