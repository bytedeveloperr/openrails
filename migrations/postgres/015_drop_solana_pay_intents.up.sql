-- Migration 015: Drop Solana Pay intents table

SET lock_timeout = '10s';
SET statement_timeout = '300s';

DROP TABLE IF EXISTS billing.solana_pay_intents;
