ALTER TABLE billing.payments
  DROP COLUMN IF EXISTS metadata;

ALTER TABLE billing.payment_methods
  DROP COLUMN IF EXISTS metadata;

