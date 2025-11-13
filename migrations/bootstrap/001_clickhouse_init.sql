-- ClickHouse Bootstrap for local development
-- User and database are created by container entrypoint via CLICKHOUSE_USER and CLICKHOUSE_DB env vars
-- This script runs during entrypoint BEFORE cluster coordination is ready, so no ON CLUSTER commands
-- System grants have been moved to migrations/clickhouse/000_system_grants.up.sql

-- Bootstrap complete - user and database created by entrypoint
-- Run 'ch-migrate up' to apply cluster-level permissions and schema
