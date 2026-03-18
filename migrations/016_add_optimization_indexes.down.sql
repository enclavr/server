-- Migration 016: Rollback optimization indexes
-- Created: 2026-03-18

DROP INDEX IF EXISTS idx_audit_logs_user_action;
DROP INDEX IF EXISTS idx_audit_logs_target_type_id;
DROP INDEX IF EXISTS idx_attachments_deleted_at;
DROP INDEX IF EXISTS idx_attachment_shares_attachment_id;
DROP INDEX IF EXISTS idx_attachment_shares_share_url;
DROP INDEX IF EXISTS idx_attachment_shares_is_active;
DROP INDEX IF EXISTS idx_categories_created_by;
DROP INDEX IF EXISTS idx_user_preferences_updated_at;
DROP INDEX IF EXISTS idx_user_statuses_status;
DROP INDEX IF EXISTS idx_scheduled_messages_scheduled_at;
DROP INDEX IF EXISTS idx_message_reminders_remind_at;
