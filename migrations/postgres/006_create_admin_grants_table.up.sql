-- Migration: Create admin_grants table for tracking admin-initiated product grants
-- This table records when admins grant products to users (comps, contest winners, manual payments, etc.)

CREATE TABLE IF NOT EXISTS billing.admin_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    price_id UUID NOT NULL REFERENCES billing.prices(id),
    granted_by TEXT NOT NULL,
    reason TEXT NOT NULL,
    payment_id UUID REFERENCES billing.payments(id),
    duration_days INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for looking up grants by user
CREATE INDEX IF NOT EXISTS idx_admin_grants_user_id ON billing.admin_grants(user_id);

-- Index for looking up grants by admin who made them
CREATE INDEX IF NOT EXISTS idx_admin_grants_granted_by ON billing.admin_grants(granted_by);

-- Index for looking up grants by payment (for audit trail)
CREATE INDEX IF NOT EXISTS idx_admin_grants_payment_id ON billing.admin_grants(payment_id) WHERE payment_id IS NOT NULL;

COMMENT ON TABLE billing.admin_grants IS 'Records admin-initiated product grants (comps, contest winners, manual payments, partnerships)';
COMMENT ON COLUMN billing.admin_grants.user_id IS 'User receiving the grant';
COMMENT ON COLUMN billing.admin_grants.price_id IS 'Price/Product being granted - entitlements derived from Product.EntitlementsSpec';
COMMENT ON COLUMN billing.admin_grants.granted_by IS 'Admin user ID who made the grant';
COMMENT ON COLUMN billing.admin_grants.reason IS 'Reason for grant: comp, contest_winner, refund_compensation, partnership, manual_payment, etc.';
COMMENT ON COLUMN billing.admin_grants.payment_id IS 'Optional link to Payment record if money was received';
COMMENT ON COLUMN billing.admin_grants.duration_days IS 'Override entitlement duration (NULL=use Product spec, 0=indefinite, N=N days)';
