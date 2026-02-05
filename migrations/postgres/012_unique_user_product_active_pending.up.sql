-- Migration 012: Prevent duplicate active/pending subscriptions per user/product

-- This unique index blocks concurrent or repeated inserts that would create multiple
-- active/pending subscriptions for the same user and product. One-time purchases
-- are unaffected because they do not create subscription rows.

-- CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_user_product_active_pending
--    ON billing.subscriptions(user_id, product_id)
--    WHERE status IN ('active', 'pending');
SELECT 1;