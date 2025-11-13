-- Revoke system table permissions
REVOKE ON CLUSTER doujins SELECT ON system.tables FROM analytics_user;
REVOKE ON CLUSTER doujins SELECT ON system.columns FROM analytics_user;
REVOKE ON CLUSTER doujins SELECT ON system.databases FROM analytics_user;
REVOKE ON CLUSTER doujins SELECT ON system.parts FROM analytics_user;
REVOKE ON CLUSTER doujins SELECT ON system.parts_columns FROM analytics_user;
REVOKE ON CLUSTER doujins SELECT ON system.mutations FROM analytics_user;
REVOKE ON CLUSTER doujins SELECT ON system.merges FROM analytics_user;
REVOKE ON CLUSTER doujins SELECT ON system.processes FROM analytics_user;
