-- 00000_baseline_roles.sql
-- Creates service roles/schemas for doujins and billing applications.
-- Variables provided by caller:
--   :DOUJINS_PW   - doujins_app password
--   :BILLING_PW   - billing_app password

\set ON_ERROR_STOP on

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'doujins_app') THEN
        CREATE ROLE doujins_app LOGIN;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'billing_app') THEN
        CREATE ROLE billing_app LOGIN;
    END IF;
END $$;

ALTER ROLE doujins_app WITH LOGIN PASSWORD :'DOUJINS_PW';
ALTER ROLE billing_app WITH LOGIN PASSWORD :'BILLING_PW';

ALTER SCHEMA public OWNER TO admin;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

CREATE SCHEMA IF NOT EXISTS doujins AUTHORIZATION doujins_app;
CREATE SCHEMA IF NOT EXISTS billing AUTHORIZATION billing_app;

SELECT current_database() AS dbname;\gset
GRANT CONNECT ON DATABASE :dbname TO doujins_app, billing_app;
GRANT CREATE ON DATABASE :dbname TO doujins_app, billing_app;

CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA doujins;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

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

GRANT USAGE ON SCHEMA doujins TO billing_app;
GRANT USAGE ON SCHEMA billing TO doujins_app;

GRANT SELECT ON ALL TABLES IN SCHEMA doujins TO billing_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA doujins TO billing_app;

GRANT SELECT ON ALL TABLES IN SCHEMA billing TO doujins_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA billing TO doujins_app;

ALTER DEFAULT PRIVILEGES FOR ROLE doujins_app IN SCHEMA doujins
  GRANT SELECT ON TABLES TO billing_app;
ALTER DEFAULT PRIVILEGES FOR ROLE doujins_app IN SCHEMA doujins
  GRANT USAGE, SELECT ON SEQUENCES TO billing_app;

ALTER DEFAULT PRIVILEGES FOR ROLE billing_app IN SCHEMA billing
  GRANT SELECT ON TABLES TO doujins_app;
ALTER DEFAULT PRIVILEGES FOR ROLE billing_app IN SCHEMA billing
  GRANT USAGE, SELECT ON SEQUENCES TO doujins_app;

ALTER DEFAULT PRIVILEGES FOR ROLE admin IN SCHEMA doujins
  GRANT SELECT ON TABLES TO doujins_app, billing_app;
ALTER DEFAULT PRIVILEGES FOR ROLE admin IN SCHEMA doujins
  GRANT USAGE, SELECT ON SEQUENCES TO doujins_app, billing_app;

GRANT SELECT ON ALL TABLES IN SCHEMA doujins TO doujins_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA doujins TO doujins_app;
