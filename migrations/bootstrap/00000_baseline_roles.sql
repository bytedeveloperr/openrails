-- 00000_baseline_roles.sql (simplified per-service roles)
-- Creates one LOGIN role per service that owns and manages its own schema (DDL + DML),
-- and grants read-only access across schemas. Intended to be run as an admin/superuser.
-- Variables provided by caller:
--   :APP_PW       - doujins_app password
--   :BILLING_PW   - billing_app password
--   :CASDOOR_PW   - casdoor_app password

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
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'casdoor_app') THEN
        CREATE ROLE casdoor_app LOGIN;
    END IF;
END $$;

ALTER ROLE doujins_app  WITH LOGIN PASSWORD :'APP_PW';
ALTER ROLE billing_app  WITH LOGIN PASSWORD :'BILLING_PW';
ALTER ROLE casdoor_app  WITH LOGIN PASSWORD :'CASDOOR_PW';

-- Harden public schema
ALTER SCHEMA public OWNER TO admin;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

-- Create schemas owned by their service roles
CREATE SCHEMA IF NOT EXISTS doujins AUTHORIZATION doujins_app;
CREATE SCHEMA IF NOT EXISTS billing AUTHORIZATION billing_app;
CREATE SCHEMA IF NOT EXISTS casdoor AUTHORIZATION casdoor_app;

-- Ensure connection privileges
-- Resolve current database name for GRANTs
SELECT current_database() AS dbname;\gset
GRANT CONNECT ON DATABASE :dbname TO doujins_app, billing_app, casdoor_app;

-- Allow service roles to create schemas if missing (needed when a service ensures its schema)
GRANT CREATE ON DATABASE :dbname TO doujins_app, billing_app, casdoor_app;

-- Enable required extensions at the database level (admin context)
-- pgvector for semantic search features
CREATE EXTENSION IF NOT EXISTS vector;
-- cryptographic and UUID helpers (gen_random_uuid)
CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- needed for range exclusion constraints
CREATE EXTENSION IF NOT EXISTS btree_gist;

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

ALTER DEFAULT PRIVILEGES FOR ROLE casdoor_app IN SCHEMA casdoor
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO casdoor_app;
ALTER DEFAULT PRIVILEGES FOR ROLE casdoor_app IN SCHEMA casdoor
  GRANT USAGE, SELECT ON SEQUENCES TO casdoor_app;
ALTER ROLE casdoor_app SET search_path = casdoor, public;

-- Cross-schema read-only: allow each role to read others' schemas
GRANT USAGE ON SCHEMA doujins, billing, casdoor TO doujins_app, billing_app, casdoor_app;

-- Grant SELECT on existing tables in each schema to the other two roles
GRANT SELECT ON ALL TABLES IN SCHEMA doujins TO billing_app, casdoor_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA doujins TO billing_app, casdoor_app;

GRANT SELECT ON ALL TABLES IN SCHEMA billing TO doujins_app, casdoor_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA billing TO doujins_app, casdoor_app;

GRANT SELECT ON ALL TABLES IN SCHEMA casdoor TO doujins_app, billing_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA casdoor TO doujins_app, billing_app;

-- Ensure future tables in each schema are readable by the other two roles
ALTER DEFAULT PRIVILEGES FOR ROLE doujins_app IN SCHEMA doujins
  GRANT SELECT ON TABLES TO billing_app, casdoor_app;
ALTER DEFAULT PRIVILEGES FOR ROLE doujins_app IN SCHEMA doujins
  GRANT USAGE, SELECT ON SEQUENCES TO billing_app, casdoor_app;

ALTER DEFAULT PRIVILEGES FOR ROLE billing_app IN SCHEMA billing
  GRANT SELECT ON TABLES TO doujins_app, casdoor_app;
ALTER DEFAULT PRIVILEGES FOR ROLE billing_app IN SCHEMA billing
  GRANT USAGE, SELECT ON SEQUENCES TO doujins_app, casdoor_app;

ALTER DEFAULT PRIVILEGES FOR ROLE casdoor_app IN SCHEMA casdoor
  GRANT SELECT ON TABLES TO doujins_app, billing_app;
ALTER DEFAULT PRIVILEGES FOR ROLE casdoor_app IN SCHEMA casdoor
  GRANT USAGE, SELECT ON SEQUENCES TO doujins_app, billing_app;

-- Done.
