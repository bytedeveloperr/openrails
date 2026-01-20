-- Migration 014 down: Best-effort reversal of entitlement source invariants changes

ALTER TABLE billing.entitlements
ALTER COLUMN source_id DROP NOT NULL;

ALTER TABLE billing.entitlements
DROP CONSTRAINT IF EXISTS chk_entitlements_source_type;

-- Re-tighten admin_grants.price_id to NOT NULL.
-- If any rows have NULL price_id, we either map them to an arbitrary existing
-- price (if one exists) or delete them (if no prices exist).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM billing.admin_grants WHERE price_id IS NULL) THEN
        IF EXISTS (SELECT 1 FROM billing.prices LIMIT 1) THEN
            UPDATE billing.admin_grants
            SET price_id = (SELECT id FROM billing.prices LIMIT 1)
            WHERE price_id IS NULL;
        ELSE
            DELETE FROM billing.admin_grants WHERE price_id IS NULL;
        END IF;
    END IF;
END$$;

ALTER TABLE billing.admin_grants
ALTER COLUMN price_id SET NOT NULL;

