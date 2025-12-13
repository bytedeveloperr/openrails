-- Migration 007: Add consistency constraints
-- These constraints prevent invalid states at the database level rather than
-- relying on application-level checks.

-- ============================================================================
-- SUBSCRIPTION CONSTRAINTS
-- ============================================================================

-- 1. Ensure cancelled subscriptions always have cancellation timestamp
-- (Prevents SS-2: cancelled_without_metadata)
ALTER TABLE billing.subscriptions
ADD CONSTRAINT chk_cancelled_has_timestamp
CHECK (status != 'cancelled' OR cancelled_at IS NOT NULL);

-- 2. Ensure cancelled subscriptions always have a cancel_type
-- (Also prevents SS-2)
ALTER TABLE billing.subscriptions
ADD CONSTRAINT chk_cancelled_has_type
CHECK (status != 'cancelled' OR cancel_type IS NOT NULL);

-- 3. Ensure subscription period dates are valid (start < end)
-- (Prevents SS-4: invalid_period_dates)
ALTER TABLE billing.subscriptions
ADD CONSTRAINT chk_valid_period
CHECK (
    current_period_starts_at IS NULL
    OR current_period_ends_at IS NULL
    OR current_period_starts_at < current_period_ends_at
);

-- 4. Ensure ended_at is not before cancelled_at
-- (Prevents SS-5: ended_before_cancelled)
ALTER TABLE billing.subscriptions
ADD CONSTRAINT chk_ended_not_before_cancelled
CHECK (
    ended_at IS NULL
    OR cancelled_at IS NULL
    OR ended_at >= cancelled_at
);

-- ============================================================================
-- ENTITLEMENT CONSTRAINTS
-- ============================================================================

-- 5. Ensure entitlement time windows are valid (start < end)
-- (Prevents ES-3: invalid_time_window)
ALTER TABLE billing.entitlements
ADD CONSTRAINT chk_valid_time_window
CHECK (end_at IS NULL OR start_at < end_at);

-- 6. Ensure revoked_at and revoke_reason are always set together
-- (Prevents ES-1: revoked_without_reason and ES-2: reason_without_revocation)
ALTER TABLE billing.entitlements
ADD CONSTRAINT chk_revoke_fields_together
CHECK ((revoked_at IS NULL) = (revoke_reason IS NULL));

-- ============================================================================
-- PAYMENT CONSTRAINTS
-- ============================================================================

-- 7. Ensure payments are not dated in the future (with 5 min grace for clock skew)
-- (Prevents T-3: future_dated_payment)
ALTER TABLE billing.payments
ADD CONSTRAINT chk_payment_not_future
CHECK (purchased_at <= NOW() + INTERVAL '5 minutes');

-- ============================================================================
-- EXISTING CONSTRAINTS (already in 001_setup_billing_tables.up.sql)
-- ============================================================================
-- These are documented here for reference but already exist:
--
-- D-1 (MultipleActiveSubscriptions): idx_subscriptions_user_active
--   UNIQUE INDEX on (user_id) WHERE status = 'active'
--
-- ES-5 (MultipleIndefiniteEntitlements): uniq_entitlements_active
--   UNIQUE INDEX on (user_id, entitlement) WHERE revoked_at IS NULL AND end_at IS NULL
--
-- D-3 (OverlappingEntitlementWindows): idx_entitlements_no_overlap
--   UNIQUE INDEX on (user_id, entitlement, start_at) WHERE revoked_at IS NULL AND deleted_at IS NULL
--   Note: This only prevents same start_at, not true range overlaps.
--   True overlap prevention would require btree_gist exclusion constraint.
--
-- FK constraints prevent orphan references:
--   fk_subscriptions_price_id, fk_subscriptions_product, fk_subscriptions_payment_method_id
