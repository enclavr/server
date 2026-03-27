-- Migration 016: Add optimization indexes for audit logs, attachments, and shares
-- Created: 2026-03-18

-- Add composite index for audit log queries by user and action
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_action 
    ON audit_logs(user_id, action DESC);

-- Add index for audit logs by target type and id
CREATE INDEX IF NOT EXISTS idx_audit_logs_target_type_id 
    ON audit_logs(target_type, target_id);

-- Add deleted_at index for soft delete performance on attachments
CREATE INDEX IF NOT EXISTS idx_attachments_deleted_at 
    ON attachments(deleted_at) WHERE deleted_at IS NULL;

-- Add index for attachment shares
CREATE INDEX IF NOT EXISTS idx_attachment_shares_attachment_id 
    ON attachment_shares(attachment_id);

CREATE INDEX IF NOT EXISTS idx_attachment_shares_share_url 
    ON attachment_shares(share_url);

CREATE INDEX IF NOT EXISTS idx_attachment_shares_is_active 
    ON attachment_shares(is_active) WHERE is_active = true;

-- Add index on categories for created_by lookup
CREATE INDEX IF NOT EXISTS idx_categories_created_by 
    ON categories(created_by);

-- Add composite index for user_preferences quick lookup
CREATE INDEX IF NOT EXISTS idx_user_preferences_updated_at 
    ON user_preferences(updated_at DESC);

-- Add index for user_statuses by status
CREATE INDEX IF NOT EXISTS idx_user_statuses_status 
    ON user_statuses(status);

-- Add index for scheduled_messages by scheduled_at
CREATE INDEX IF NOT EXISTS idx_scheduled_messages_scheduled_at 
    ON scheduled_messages(scheduled_at) WHERE is_sent = false AND is_cancelled = false;

-- Add index for message_reminders by remind_at
CREATE INDEX IF NOT EXISTS idx_message_reminders_remind_at 
    ON message_reminders(remind_at) WHERE is_triggered = false;
