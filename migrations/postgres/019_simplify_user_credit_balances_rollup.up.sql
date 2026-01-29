ALTER TABLE billing.user_credit_balances
  DROP COLUMN IF EXISTS permanent_balance,
  DROP COLUMN IF EXISTS expiring_balance,
  DROP COLUMN IF EXISTS earliest_expiry;

