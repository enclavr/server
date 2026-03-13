-- +migrate Down
DROP TABLE IF EXISTS user_notifications CASCADE;
DROP TABLE IF EXISTS user_roles CASCADE;
DROP TABLE IF EXISTS role_permissions CASCADE;
DROP TABLE IF EXISTS roles CASCADE;
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS audit_log_exclusions CASCADE;
DROP TABLE IF EXISTS user_devices CASCADE;
DROP TABLE IF EXISTS category_permissions CASCADE;
DROP TABLE IF EXISTS attachments CASCADE;

DROP INDEX IF EXISTS idx_audit_logs_user_action_created;
DROP INDEX IF EXISTS idx_messages_room_created_deleted;
