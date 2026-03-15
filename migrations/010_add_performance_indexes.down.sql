-- +migrate Down
-- Rollback performance indexes

DROP INDEX IF EXISTS idx_audit_logs_user_action;
DROP INDEX IF EXISTS idx_message_reactions_message_user;
DROP INDEX IF EXISTS idx_user_status_status;
DROP INDEX IF EXISTS idx_attachments_message_created;
DROP INDEX IF EXISTS idx_poll_votes_option_user;
DROP INDEX IF EXISTS idx_scheduled_messages_room_scheduled;
DROP INDEX IF EXISTS idx_message_reminders_user_triggered;
