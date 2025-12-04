-- Migration: Drop gateway columns
-- The gateway field is redundant since processor (mobius, ccbill, solana)
-- already determines the underlying gateway (NMI for mobius, self-contained for others)

-- Drop gateway column from subscriptions
ALTER TABLE billing.subscriptions DROP COLUMN IF EXISTS gateway;

-- Drop gateway column from payments
ALTER TABLE billing.payments DROP COLUMN IF EXISTS gateway;

-- Drop gateway column from payment_methods
ALTER TABLE billing.payment_methods DROP COLUMN IF EXISTS gateway;

-- Note: 'nmi' was never a valid enum value for processor_type, so no data migration needed.
-- The enum only contains: 'paypal', 'solana', 'mobius', 'ccbill'
