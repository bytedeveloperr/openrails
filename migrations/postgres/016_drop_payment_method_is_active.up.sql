-- Migration 016: Remove is_active from payment_methods
-- Payment methods now just exist or don't exist - no active/inactive concept.

ALTER TABLE billing.payment_methods
DROP COLUMN IF EXISTS is_active;
