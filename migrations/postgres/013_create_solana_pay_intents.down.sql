-- Down migration for 013: drop Solana Pay intents table

DROP TABLE IF EXISTS billing.solana_pay_intents;
