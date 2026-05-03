ALTER TABLE billing.entitlements
    DROP CONSTRAINT IF EXISTS entitlements_no_overlap;

CREATE UNIQUE INDEX IF NOT EXISTS idx_entitlements_no_overlap
    ON billing.entitlements(user_id, entitlement, start_at)
    WHERE revoked_at IS NULL AND deleted_at IS NULL;
