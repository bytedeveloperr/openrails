-- PostgreSQL Bootstrap
-- Simple initialization: create schemas and install extensions
-- All apps use the admin super-user

-- Create schemas
CREATE SCHEMA IF NOT EXISTS doujins;
CREATE SCHEMA IF NOT EXISTS billing;
CREATE SCHEMA IF NOT EXISTS hentai0;
CREATE SCHEMA IF NOT EXISTS profiles;

-- Install required extensions in public schema
CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS citext WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public;
