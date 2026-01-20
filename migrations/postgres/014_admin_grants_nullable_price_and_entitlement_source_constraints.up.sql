-- Migration 014: Make admin_grants.price_id optional and enforce entitlement source invariants
--
-- Requirements:
-- - Admin-issued entitlements must have a concrete source record (admin_grants.id).
-- - Entitlements must always have source_id and only allow the supported source types.

-- 1) Allow admin grants to exist without a product/price (pure admin comps/support overrides)
ALTER TABLE billing.admin_grants
ALTER COLUMN price_id DROP NOT NULL;

-- 2) Backfill any historical admin entitlements that have source_id IS NULL
-- We create one synthetic admin_grants row per user, and link all missing-source
-- admin entitlements for that user to it.
WITH users_to_fix AS (
    SELECT DISTINCT user_id::text AS user_id
    FROM billing.entitlements
    WHERE source_type = 'admin' AND source_id IS NULL
),
inserted AS (
    INSERT INTO billing.admin_grants (id, user_id, price_id, granted_by, reason, payment_id, duration_days, created_at)
    SELECT gen_random_uuid(), user_id, NULL, 'migration', 'backfill_missing_entitlement_source', NULL, NULL, NOW()
    FROM users_to_fix
    RETURNING id, user_id
)
UPDATE billing.entitlements e
SET source_id = i.id
FROM inserted i
WHERE e.source_type = 'admin'
  AND e.source_id IS NULL
  AND e.user_id::text = i.user_id;

-- 3) Enforce allowed source types and non-null source_id for all entitlements
ALTER TABLE billing.entitlements
ADD CONSTRAINT chk_entitlements_source_type
CHECK (source_type IN ('subscription', 'one_off', 'admin'));

ALTER TABLE billing.entitlements
ALTER COLUMN source_id SET NOT NULL;

