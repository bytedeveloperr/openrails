-- PostgreSQL Bootstrap
-- Simple initialization: create required billing schema and install extensions.
-- Open Rails Billing is designed to run standalone; do not create schemas for other apps here.

-- Create schemas
CREATE SCHEMA IF NOT EXISTS billing;

-- Install required extensions in public schema.
-- Billing currently relies on pgcrypto for gen_random_uuid().
CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;

-- Create migratekit migrations tracking table
-- Note: Locking is now handled via PostgreSQL advisory locks, not this table
CREATE TABLE IF NOT EXISTS public.migrations (
    id BIGSERIAL PRIMARY KEY,
    app TEXT NOT NULL,
    database TEXT NOT NULL,
    name TEXT NOT NULL,
    migrated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(app, database, name)
);
