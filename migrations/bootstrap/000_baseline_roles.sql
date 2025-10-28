-- 00000_baseline_roles.sql (simplified per-service roles)
-- Creates one LOGIN role per service that owns and manages its own schema (DDL + DML),
-- and grants read-only access across schemas. Intended to be run as an admin/superuser.
-- Variables provided by caller:
--   :DOUJINS_PW   - doujins_app password
--   :BILLING_PW   - billing_app password
--   :HENTAI0_PW   - hentai0_app password

\set ON_ERROR_STOP on

-- Use psql variable substitution and \gexec to avoid DO-block quoting issues.
-- Create/alter service roles with provided passwords.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'doujins_app') THEN
        CREATE ROLE doujins_app LOGIN;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'billing_app') THEN
        CREATE ROLE billing_app LOGIN;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hentai0_app') THEN
        CREATE ROLE hentai0_app LOGIN;
    END IF;
END $$;

ALTER ROLE doujins_app  WITH LOGIN PASSWORD :'DOUJINS_PW';
ALTER ROLE billing_app  WITH LOGIN PASSWORD :'BILLING_PW';
ALTER ROLE hentai0_app  WITH LOGIN PASSWORD :'HENTAI0_PW';

-- Harden public schema
ALTER SCHEMA public OWNER TO admin;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

-- Create schemas owned by their service roles
CREATE SCHEMA IF NOT EXISTS doujins AUTHORIZATION doujins_app;
CREATE SCHEMA IF NOT EXISTS billing AUTHORIZATION billing_app;
CREATE SCHEMA IF NOT EXISTS hentai0 AUTHORIZATION hentai0_app;

-- Ensure connection privileges
-- Resolve current database name for GRANTs
SELECT current_database() AS dbname;\gset
GRANT CONNECT ON DATABASE :dbname TO doujins_app, billing_app, hentai0_app;

-- Allow service roles to create schemas if missing (needed when a service ensures its schema)
GRANT CREATE ON DATABASE :dbname TO doujins_app, billing_app, hentai0_app;

-- Core extensions needed by the app are created during bootstrap, not in app migrations
-- App-specific extensions stay in the app schema; global ones stay in public.
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA doujins;
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA doujins;
-- pgcrypto and citext are used by multiple schemas (incl. authkit). Install globally.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA doujins;

-- Default privileges: service roles own and can fully manage their own objects
ALTER DEFAULT PRIVILEGES FOR ROLE doujins_app IN SCHEMA doujins
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO doujins_app;
ALTER DEFAULT PRIVILEGES FOR ROLE doujins_app IN SCHEMA doujins
  GRANT USAGE, SELECT ON SEQUENCES TO doujins_app;
ALTER ROLE doujins_app SET search_path = doujins, public;

ALTER DEFAULT PRIVILEGES FOR ROLE billing_app IN SCHEMA billing
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO billing_app;
ALTER DEFAULT PRIVILEGES FOR ROLE billing_app IN SCHEMA billing
  GRANT USAGE, SELECT ON SEQUENCES TO billing_app;
ALTER ROLE billing_app SET search_path = billing, public;

ALTER DEFAULT PRIVILEGES FOR ROLE hentai0_app IN SCHEMA hentai0
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO hentai0_app;
ALTER DEFAULT PRIVILEGES FOR ROLE hentai0_app IN SCHEMA hentai0
  GRANT USAGE, SELECT ON SEQUENCES TO hentai0_app;
ALTER ROLE hentai0_app SET search_path = hentai0, public;


-- Cross-schema read-only: allow each role to read others' schemas
GRANT USAGE ON SCHEMA doujins, billing, hentai0 TO doujins_app, billing_app, hentai0_app;

-- Grant SELECT on existing tables in each schema to the other roles
GRANT SELECT ON ALL TABLES IN SCHEMA doujins TO billing_app, hentai0_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA doujins TO billing_app, hentai0_app;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA billing TO doujins_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA billing TO doujins_app;

GRANT SELECT ON ALL TABLES IN SCHEMA hentai0 TO doujins_app, billing_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA hentai0 TO doujins_app, billing_app;

-- Ensure future tables in each schema are readable by the other roles
ALTER DEFAULT PRIVILEGES FOR ROLE doujins_app IN SCHEMA doujins
  GRANT SELECT ON TABLES TO billing_app, hentai0_app;
ALTER DEFAULT PRIVILEGES FOR ROLE doujins_app IN SCHEMA doujins
  GRANT USAGE, SELECT ON SEQUENCES TO billing_app, hentai0_app;

ALTER DEFAULT PRIVILEGES FOR ROLE billing_app IN SCHEMA billing
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO doujins_app;
ALTER DEFAULT PRIVILEGES FOR ROLE billing_app IN SCHEMA billing
  GRANT USAGE, SELECT ON SEQUENCES TO doujins_app;

ALTER DEFAULT PRIVILEGES FOR ROLE hentai0_app IN SCHEMA hentai0
  GRANT SELECT ON TABLES TO doujins_app, billing_app;
ALTER DEFAULT PRIVILEGES FOR ROLE hentai0_app IN SCHEMA hentai0
  GRANT USAGE, SELECT ON SEQUENCES TO doujins_app, billing_app;


-- Ensure future tables created by admin in doujins schema are accessible to service roles
-- This covers migration system tables like migration_progress
ALTER DEFAULT PRIVILEGES FOR ROLE admin IN SCHEMA doujins
  GRANT SELECT ON TABLES TO doujins_app, billing_app, hentai0_app;
ALTER DEFAULT PRIVILEGES FOR ROLE admin IN SCHEMA doujins
  GRANT USAGE, SELECT ON SEQUENCES TO doujins_app, billing_app, hentai0_app;

-- Grant access to any existing tables created by admin in doujins schema
GRANT SELECT ON ALL TABLES IN SCHEMA doujins TO doujins_app, hentai0_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA doujins TO doujins_app, hentai0_app;

-- Done.

-- AuthKit manages its own schema + privileges in its bootstrap; we do not grant here.
