-- Migration: Drop gateway columns
-- The gateway field is redundant since processor (mobius, ccbill, solana)
-- already determines the underlying gateway (NMI for mobius, self-contained for others)

-- Drop gateway column from subscriptions
ALTER TABLE billing.subscriptions DROP COLUMN IF EXISTS gateway;

-- Drop gateway column from payments
ALTER TABLE billing.payments DROP COLUMN IF EXISTS gateway;

-- Drop gateway column from payment_methods
ALTER TABLE billing.payment_methods DROP COLUMN IF EXISTS gateway;

-- Normalize any 'nmi' processor values to 'mobius'
-- (legacy data cleanup - NMI is the gateway, mobius is the processor)
UPDATE billing.subscriptions SET processor = 'mobius' WHERE processor = 'nmi';
UPDATE billing.payments SET processor = 'mobius' WHERE processor = 'nmi';
UPDATE billing.payment_methods SET processor = 'mobius' WHERE processor = 'nmi';
