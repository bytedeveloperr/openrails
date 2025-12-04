-- Rollback: Restore gateway columns

-- Add gateway column back to subscriptions
ALTER TABLE billing.subscriptions ADD COLUMN IF NOT EXISTS gateway VARCHAR(50);

-- Add gateway column back to payments
ALTER TABLE billing.payments ADD COLUMN IF NOT EXISTS gateway VARCHAR(50);

-- Add gateway column back to payment_methods
ALTER TABLE billing.payment_methods ADD COLUMN IF NOT EXISTS gateway VARCHAR(50);

-- Note: The nmi -> mobius processor normalization cannot be reversed
-- as we don't know which records were originally 'nmi'
