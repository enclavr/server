-- Migration 020: Add query optimization indexes
-- Date: 2026-03-21
-- Purpose: Add missing indexes for common query patterns

-- AuditLog: Add composite index for date range queries with action filter
CREATE INDEX IF NOT EXISTS idx_audit_log_action_created 
ON audit_logs(action, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_log_user_created 
ON audit_logs(user_id, created_at DESC);

-- Message: Add index on created_at for time-based queries
CREATE INDEX IF NOT EXISTS idx_messages_created_at 
ON messages(created_at DESC) WHERE is_deleted = FALSE;

-- Message: Add composite index for room + user queries
CREATE INDEX IF NOT EXISTS idx_messages_room_user 
ON messages(room_id, user_id, created_at DESC) WHERE is_deleted = FALSE;

-- DirectMessage: Add indexes for time-based sorting
CREATE INDEX IF NOT EXISTS idx_direct_messages_sender_created 
ON direct_messages(sender_id, created_at DESC) WHERE is_deleted = FALSE;

CREATE INDEX IF NOT EXISTS idx_direct_messages_receiver_created 
ON direct_messages(receiver_id, created_at DESC) WHERE is_deleted = FALSE;

-- Report: Add index on status for filtering
CREATE INDEX IF NOT EXISTS idx_reports_status 
ON reports(status, created_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_reports_status_reviewed 
ON reports(status, reviewed_by, created_at DESC) WHERE deleted_at IS NULL;

-- Notification: Add composite index for user + is_read (very common query)
CREATE INDEX IF NOT EXISTS idx_notifications_user_read 
ON notifications(user_id, is_read, created_at DESC) WHERE archived = FALSE;

-- Ban: Add index on expires_at for cleanup queries
CREATE INDEX IF NOT EXISTS idx_bans_expires_at 
ON bans(expires_at) WHERE deleted_at IS NULL AND expires_at IS NOT NULL;

-- ScheduledMessage: Add composite index for pending messages
CREATE INDEX IF NOT EXISTS idx_scheduled_messages_pending 
ON scheduled_messages(user_id, send_at) WHERE is_sent = FALSE AND is_cancelled = FALSE AND deleted_at IS NULL;

-- UserStatusModel: Add index on expires_at for status expiry
CREATE INDEX IF NOT EXISTS idx_user_status_expires 
ON user_statuses(expires_at) WHERE expires_at IS NOT NULL;

-- MessageRead: Add composite index for user + room
CREATE INDEX IF NOT EXISTS idx_message_read_user_room 
ON message_reads(user_id, room_id, read_at DESC);

-- UserActivityLog: Add index on created_at for cleanup queries
CREATE INDEX IF NOT EXISTS idx_user_activity_log_created 
ON user_activity_logs(created_at DESC);

-- WebhookLog: Add index on created_at for cleanup queries
CREATE INDEX IF NOT EXISTS idx_webhook_logs_created 
ON webhook_logs(created_at DESC);

-- Thread: Add index on parent_id for thread queries
CREATE INDEX IF NOT EXISTS idx_threads_parent 
ON threads(parent_id, created_at DESC) WHERE deleted_at IS NULL;

-- ThreadMessage: Add index on created_at for sorting
CREATE INDEX IF NOT EXISTS idx_thread_messages_created 
ON thread_messages(thread_id, created_at DESC) WHERE is_deleted = FALSE;

-- Poll: Add index on expires_at for active polls
CREATE INDEX IF NOT EXISTS idx_polls_active 
ON polls(expires_at) WHERE deleted_at IS NULL AND expires_at IS NOT NULL;

-- Invite: Add index on expires_at for cleanup
CREATE INDEX IF NOT EXISTS idx_invites_expires 
ON invites(expires_at) WHERE is_revoked = FALSE AND deleted_at IS NULL;

-- InviteLink: Add index on expires_at
CREATE INDEX IF NOT EXISTS idx_invite_links_expires 
ON invite_links(expires_at) WHERE is_enabled = TRUE AND deleted_at IS NULL;

-- APIKey: Add index on expires_at for cleanup
CREATE INDEX IF NOT EXISTS idx_api_keys_expires 
ON api_keys(expires_at) WHERE is_active = TRUE AND deleted_at IS NULL;

-- OAuthAccount: Add index on provider + provider_id
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_provider 
ON oauth_accounts(provider, provider_id) WHERE is_active = TRUE AND deleted_at IS NULL;
