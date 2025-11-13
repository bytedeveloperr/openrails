-- ClickHouse Bootstrap for local development
-- Uses hardcoded default password
-- Production bootstrap is in ~/doujins-gitops with Vault templating

CREATE DATABASE IF NOT EXISTS analytics ON CLUSTER doujins;

CREATE USER IF NOT EXISTS analytics_user ON CLUSTER doujins
  IDENTIFIED WITH plaintext_password BY 'analytics_password'
  HOST ANY;

GRANT ON CLUSTER doujins ALL ON analytics.* TO analytics_user;

GRANT ON CLUSTER doujins SELECT ON system.tables TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.columns TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.databases TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.parts TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.parts_columns TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.mutations TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.merges TO analytics_user;
GRANT ON CLUSTER doujins SELECT ON system.processes TO analytics_user;
