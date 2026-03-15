-- +migrate Up
-- Add performance indexes for query optimization

-- Composite index for audit logs filtering by user and action
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_action ON audit_logs(user_id, action, created_at DESC);

-- Composite index for message reactions to prevent duplicate reactions
CREATE INDEX IF NOT EXISTS idx_message_reactions_message_user ON message_reactions(message_id, user_id);

-- Composite index for user status lookups
CREATE INDEX IF NOT EXISTS idx_user_status_status ON user_statuses(status, updated_at DESC);

-- Composite index for attachment queries by message with time ordering
CREATE INDEX IF NOT EXISTS idx_attachments_message_created ON attachments(message_id, created_at DESC);

-- Composite index for poll vote lookups
CREATE INDEX IF NOT EXISTS idx_poll_votes_option_user ON poll_votes(option_id, user_id);

-- Partial index for scheduled messages pending delivery
CREATE INDEX IF NOT EXISTS idx_scheduled_messages_room_scheduled ON scheduled_messages(room_id, scheduled_at ASC) WHERE is_sent = false AND is_cancelled = false;

-- Partial index for pending message reminders
CREATE INDEX IF NOT EXISTS idx_message_reminders_user_triggered ON message_reminders(user_id, remind_at ASC) WHERE is_triggered = false;

-- +migrate Down
DROP INDEX IF EXISTS idx_audit_logs_user_action;
DROP INDEX IF EXISTS idx_message_reactions_message_user;
DROP INDEX IF EXISTS idx_user_status_status;
DROP INDEX IF EXISTS idx_attachments_message_created;
DROP INDEX IF EXISTS idx_poll_votes_option_user;
DROP INDEX IF EXISTS idx_scheduled_messages_room_scheduled;
DROP INDEX IF EXISTS idx_message_reminders_user_triggered;
