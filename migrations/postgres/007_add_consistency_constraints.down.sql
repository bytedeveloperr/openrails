-- Migration 007: Remove consistency constraints

ALTER TABLE billing.subscriptions DROP CONSTRAINT IF EXISTS chk_cancelled_has_timestamp;
ALTER TABLE billing.subscriptions DROP CONSTRAINT IF EXISTS chk_cancelled_has_type;
ALTER TABLE billing.subscriptions DROP CONSTRAINT IF EXISTS chk_valid_period;
ALTER TABLE billing.subscriptions DROP CONSTRAINT IF EXISTS chk_ended_not_before_cancelled;
ALTER TABLE billing.entitlements DROP CONSTRAINT IF EXISTS chk_valid_time_window;
ALTER TABLE billing.entitlements DROP CONSTRAINT IF EXISTS chk_revoke_fields_together;
ALTER TABLE billing.payments DROP CONSTRAINT IF EXISTS chk_payment_not_future;
