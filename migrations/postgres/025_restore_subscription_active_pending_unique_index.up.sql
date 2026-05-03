-- Migration 025: Restore active/pending subscription uniqueness for databases
-- where migration 012 was previously applied as a no-op.

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_user_product_active_pending
    ON billing.subscriptions(user_id, product_id)
    WHERE status IN ('active', 'pending');
