SET lock_timeout = '10s';
SET statement_timeout = '300s';

ALTER TABLE billing.entitlements
  DROP CONSTRAINT IF EXISTS chk_entitlements_source_type;

ALTER TABLE billing.entitlements
  ADD CONSTRAINT chk_entitlements_source_type
  CHECK (source_type IN ('subscription', 'one_off', 'admin'));

