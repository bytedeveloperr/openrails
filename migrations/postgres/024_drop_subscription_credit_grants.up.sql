SET lock_timeout = '10s';
SET statement_timeout = '300s';

-- subscription_credit_grants was an idempotency ledger for recurring subscription credit grants.
-- It has been replaced by deterministic deposit SourceIDs, so the table is no longer needed.
DROP TABLE IF EXISTS billing.subscription_credit_grants;

