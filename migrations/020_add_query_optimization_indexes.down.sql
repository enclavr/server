-- Migration 020: Rollback query optimization indexes
-- Date: 2026-03-21

DROP INDEX IF EXISTS idx_audit_log_action_created;
DROP INDEX IF EXISTS idx_audit_log_user_created;
DROP INDEX IF EXISTS idx_messages_created_at;
DROP INDEX IF EXISTS idx_messages_room_user;
DROP INDEX IF EXISTS idx_direct_messages_sender_created;
DROP INDEX IF EXISTS idx_direct_messages_receiver_created;
DROP INDEX IF EXISTS idx_reports_status;
DROP INDEX IF EXISTS idx_reports_status_reviewed;
DROP INDEX IF EXISTS idx_notifications_user_read;
DROP INDEX IF EXISTS idx_bans_expires_at;
DROP INDEX IF EXISTS idx_scheduled_messages_pending;
DROP INDEX IF EXISTS idx_user_status_expires;
DROP INDEX IF EXISTS idx_message_read_user_room;
DROP INDEX IF EXISTS idx_user_activity_log_created;
DROP INDEX IF EXISTS idx_webhook_logs_created;
DROP INDEX IF EXISTS idx_threads_parent;
DROP INDEX IF EXISTS idx_thread_messages_created;
DROP INDEX IF EXISTS idx_polls_active;
DROP INDEX IF EXISTS idx_invites_expires;
DROP INDEX IF EXISTS idx_invite_links_expires;
DROP INDEX IF EXISTS idx_api_keys_expires;
DROP INDEX IF EXISTS idx_oauth_accounts_provider;
