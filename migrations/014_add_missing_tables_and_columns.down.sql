-- +migrate Down
-- Rollback missing tables and columns

-- Drop columns from audit_logs
ALTER TABLE audit_logs DROP COLUMN IF EXISTS old_value;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS new_value;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS user_agent;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS success;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS error_message;

-- Drop tables
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS message_reads;
DROP TABLE IF EXISTS user_statuses;
DROP TABLE IF EXISTS category_permissions;
DROP TABLE IF EXISTS user_devices;
