-- Down migration for 012: drop the partial unique index enforcing one active/pending subscription per user+product.

DROP INDEX IF EXISTS idx_subscriptions_user_product_active_pending;
