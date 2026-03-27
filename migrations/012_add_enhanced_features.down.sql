-- +migrate Down
-- Drop attachment_shares table
DROP INDEX IF EXISTS idx_attachment_share_url;
DROP INDEX IF EXISTS idx_attachment_share_attachment;
DROP INDEX IF EXISTS idx_attachment_share_active;
DROP TABLE IF EXISTS attachment_shares;

-- Remove new columns from attachments
ALTER TABLE attachments DROP COLUMN IF EXISTS is_shared;
ALTER TABLE attachments DROP COLUMN IF EXISTS share_count;
ALTER TABLE attachments DROP COLUMN IF EXISTS download_count;
ALTER TABLE attachments DROP COLUMN IF EXISTS view_count;

-- Remove new columns from user_preferences
ALTER TABLE user_preferences DROP COLUMN IF EXISTS notification_sound;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS desktop_notification;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS mobile_notification;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS mention_notification;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS dm_notification;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS show_typing_indicator;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS show_read_receipts;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS auto_scroll_messages;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS use_24_hour_format;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS display_mode;

-- Remove new columns from audit_logs
ALTER TABLE audit_logs DROP COLUMN IF EXISTS old_value;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS new_value;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS user_agent;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS success;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS error_message;

-- Remove new columns from categories
ALTER TABLE categories DROP COLUMN IF EXISTS description;
ALTER TABLE categories DROP COLUMN IF EXISTS icon;
ALTER TABLE categories DROP COLUMN IF EXISTS color;
ALTER TABLE categories DROP COLUMN IF EXISTS is_private;
ALTER TABLE categories DROP COLUMN IF EXISTS created_by;

-- Drop additional performance indexes
DROP INDEX IF EXISTS idx_audit_logs_target;
DROP INDEX IF EXISTS idx_audit_logs_created_at;
DROP INDEX IF EXISTS idx_user_preferences_theme;
DROP INDEX IF EXISTS idx_categories_color;
DROP INDEX IF EXISTS idx_attachments_user_created;
