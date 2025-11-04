-- 00000_baseline_roles.sql (simplified per-service roles)
-- Creates one LOGIN role per service that owns and manages its own schema (DDL + DML),
-- and grants read-only access across schemas. Intended to be run as an admin/superuser.
-- Variables provided by caller:
--   :DOUJINS_PW   - doujins_app password
--   :BILLING_PW   - billing_app password
--   :HENTAI0_PW   - hentai0_app password

\set ON_ERROR_STOP on

-- Create/alter service roles with provided passwords
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

-- Harden public schema (but allow service roles to create tables for migratekit)
ALTER SCHEMA public OWNER TO admin;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

-- Create a neutral owner for AuthKit-managed identity objects
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authkit_owner') THEN
        CREATE ROLE authkit_owner NOLOGIN; -- group/owner role, not used for login
    END IF;
END $$;

-- Set search_path for authkit_owner to include public (for extensions like citext)
ALTER ROLE authkit_owner SET search_path = profiles, public;

-- Explicitly grant USAGE on public schema to authkit_owner (for extensions like citext)
GRANT USAGE ON SCHEMA public TO authkit_owner;

-- Create schemas; app schemas owned by app roles; profiles owned by neutral owner
CREATE SCHEMA IF NOT EXISTS doujins AUTHORIZATION doujins_app;
CREATE SCHEMA IF NOT EXISTS billing AUTHORIZATION billing_app;
CREATE SCHEMA IF NOT EXISTS hentai0 AUTHORIZATION hentai0_app;
CREATE SCHEMA IF NOT EXISTS profiles AUTHORIZATION authkit_owner;

-- Create global extensions needed by AuthKit BEFORE any service migrations run
-- Explicitly install in public schema to avoid race conditions
CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS citext WITH SCHEMA public;

-- Grant CREATE on public schema to service roles so migratekit can create its tables
-- migratekit will create public.migrations and public.migration_locks on first run
GRANT CREATE ON SCHEMA public TO doujins_app, billing_app, hentai0_app, authkit_owner;

-- Grant permissions on profiles schema so services can run AuthKit migrations safely
-- AuthKit migrations will create their own tracking tables (profiles.bun_migrations, etc.)
-- Apps can create objects; ownership will be transferred to authkit_owner in migrations
GRANT USAGE ON SCHEMA profiles TO doujins_app, billing_app, hentai0_app;
GRANT CREATE ON SCHEMA profiles TO doujins_app, billing_app, hentai0_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA profiles TO doujins_app, billing_app, hentai0_app;
GRANT REFERENCES ON ALL TABLES IN SCHEMA profiles TO doujins_app, billing_app, hentai0_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA profiles TO doujins_app, billing_app, hentai0_app;

-- Make app roles members of the neutral owner so they can SET ROLE if needed (optional)
GRANT authkit_owner TO doujins_app;
GRANT authkit_owner TO billing_app;
GRANT authkit_owner TO hentai0_app;

-- Default privileges for future tables in profiles schema (created by any service or admin)
-- All services AND authkit_owner need full CRUD on all profiles schema objects
DO $$
DECLARE
    role_name TEXT;
BEGIN
    FOREACH role_name IN ARRAY ARRAY['authkit_owner', 'doujins_app', 'billing_app', 'hentai0_app', 'admin']
    LOOP
        EXECUTE format('
            ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA profiles
              GRANT SELECT, INSERT, UPDATE, DELETE, REFERENCES ON TABLES TO authkit_owner, doujins_app, billing_app, hentai0_app;
            ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA profiles
              GRANT USAGE, SELECT ON SEQUENCES TO authkit_owner, doujins_app, billing_app, hentai0_app;
        ', role_name, role_name);
    END LOOP;
END $$;

-- Ensure connection privileges
SELECT current_database() AS dbname;\gset
GRANT CONNECT ON DATABASE :dbname TO doujins_app, billing_app, hentai0_app;
GRANT CREATE ON DATABASE :dbname TO doujins_app, billing_app, hentai0_app;

-- Core extensions needed by the app (global extensions in public schema for shared use)
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public;

-- Default privileges: service roles own and can fully manage their own objects
DO $$
DECLARE
    service RECORD;
BEGIN
    FOR service IN
        SELECT unnest(ARRAY['doujins', 'billing', 'hentai0']) as schema_name,
               unnest(ARRAY['doujins_app', 'billing_app', 'hentai0_app']) as role_name
    LOOP
        EXECUTE format('
            ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA %I
              GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %I;
            ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA %I
              GRANT USAGE, SELECT ON SEQUENCES TO %I;
            ALTER ROLE %I SET search_path = %I, public;
        ', service.role_name, service.schema_name, service.role_name,
           service.role_name, service.schema_name, service.role_name,
           service.role_name, service.schema_name);
    END LOOP;
END $$;

-- Cross-schema read-only: allow each role to read others' schemas
GRANT USAGE ON SCHEMA doujins, billing, hentai0 TO doujins_app, billing_app, hentai0_app;

-- Grant SELECT on existing tables in each schema to the other roles
GRANT SELECT ON ALL TABLES IN SCHEMA doujins TO billing_app, hentai0_app, doujins_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA doujins TO billing_app, hentai0_app, doujins_app;

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
ALTER DEFAULT PRIVILEGES FOR ROLE admin IN SCHEMA doujins
  GRANT SELECT ON TABLES TO doujins_app, billing_app, hentai0_app;
ALTER DEFAULT PRIVILEGES FOR ROLE admin IN SCHEMA doujins
  GRANT USAGE, SELECT ON SEQUENCES TO doujins_app, billing_app, hentai0_app;

-- Done. AuthKit manages its own schema + privileges in its bootstrap.
