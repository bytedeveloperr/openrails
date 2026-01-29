SET lock_timeout = '10s';
SET statement_timeout = '300s';

DROP TABLE IF EXISTS billing.subscription_credit_grants;

DROP INDEX IF EXISTS billing.idx_subscriptions_grace_ends_at;

ALTER TABLE billing.subscriptions
  DROP COLUMN IF EXISTS grace_ends_at;
