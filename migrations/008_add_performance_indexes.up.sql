-- +migrate Up
-- Additional performance indexes and model enhancements

-- Add composite index for audit_logs for admin queries
CREATE INDEX IF NOT EXISTS idx_audit_logs_action_created ON audit_logs (action, created_at DESC);

-- Add composite index for audit_logs for target-based queries
CREATE INDEX IF NOT EXISTS idx_audit_logs_target ON audit_logs (target_type, target_id);

-- Add composite index for user_preferences for common lookup patterns
CREATE INDEX IF NOT EXISTS idx_user_preferences_user_id ON user_preferences (user_id);

-- Add composite index for categories for sorted listing
CREATE INDEX IF NOT EXISTS idx_categories_sort_order ON categories (sort_order, deleted_at) WHERE deleted_at IS NULL;

-- Add composite index for files for room file queries
CREATE INDEX IF NOT EXISTS idx_files_room_created ON files (room_id, created_at DESC) WHERE is_deleted = false;

-- Add composite index for files for user file queries
CREATE INDEX IF NOT EXISTS idx_files_user_created ON files (user_id, created_at DESC) WHERE is_deleted = false;

-- Add composite index for messages for search queries
CREATE INDEX IF NOT EXISTS idx_messages_user_created ON messages (user_id, created_at DESC) WHERE is_deleted = false;

-- Add composite index for invites for room invite lookups
CREATE INDEX IF NOT EXISTS idx_invites_room_code ON invites (room_id, code) WHERE is_revoked = false AND deleted_at IS NULL;

-- Add composite index for message_read for unread counts
CREATE INDEX IF NOT EXISTS idx_message_read_user_room ON message_read (user_id, room_id, read_at DESC);

-- Add composite index for bans for room ban lookups
CREATE INDEX IF NOT EXISTS idx_bans_room_user ON bans (room_id, user_id) WHERE deleted_at IS NULL;

-- Add composite index for reports for status filtering
CREATE INDEX IF NOT EXISTS idx_reports_status_created ON reports (status, created_at DESC);

-- Add composite index for reports for reporter lookups
CREATE INDEX IF NOT EXISTS idx_reports_reporter ON reports (reporter_id, created_at DESC);

-- Add composite index for reports for reported user lookups
CREATE INDEX IF NOT EXISTS idx_reports_reported ON reports (reported_id, created_at DESC);

-- Add composite index for webhooks for room webhook lookups
CREATE INDEX IF NOT EXISTS idx_webhooks_room_active ON webhooks (room_id, is_active) WHERE is_active = true AND deleted_at IS NULL;

-- Add composite index for webhooks logs for webhook lookups
CREATE INDEX IF NOT EXISTS idx_webhook_logs_webhook_created ON webhook_logs (webhook_id, created_at DESC);

-- Add composite index for sessions for user session cleanup
CREATE INDEX IF NOT EXISTS idx_sessions_user_expires ON sessions (user_id, expires_at) WHERE deleted_at IS NULL;

-- Add composite index for refresh_tokens for user token lookups
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens (user_id, created_at DESC);

-- Add composite index for blocks for blocked user lookups
CREATE INDEX IF NOT EXISTS idx_blocks_blocker ON blocks (blocker_id, created_at DESC);

-- Add composite index for threads for parent message lookups
CREATE INDEX IF NOT EXISTS idx_threads_parent ON threads (parent_id, deleted_at) WHERE deleted_at IS NULL;

-- Add composite index for thread_messages for thread lookups
CREATE INDEX IF NOT EXISTS idx_thread_messages_thread ON thread_messages (thread_id, created_at DESC) WHERE is_deleted = false;

-- Add composite index for polls for room poll lookups
CREATE INDEX IF NOT EXISTS idx_polls_room_active ON polls (room_id, created_at DESC) WHERE deleted_at IS NULL;

-- Add composite index for polls for user poll lookups
CREATE INDEX IF NOT EXISTS idx_polls_user ON polls (created_by, created_at DESC);

-- Add composite index for poll_votes for poll result lookups
CREATE INDEX IF NOT EXISTS idx_poll_votes_option ON poll_votes (option_id, user_id);

-- Add composite index for server_emoji for emoji listing
CREATE INDEX IF NOT EXISTS idx_server_emoji_created ON server_emoji (created_at DESC) WHERE deleted_at IS NULL;

-- Add composite index for server_sticker for sticker listing
CREATE INDEX IF NOT EXISTS idx_server_sticker_created ON server_sticker (created_at DESC) WHERE deleted_at IS NULL;

-- Add composite index for soundboard_sounds for sound listing
CREATE INDEX IF NOT EXISTS idx_soundboard_sounds_created ON soundboard_sounds (created_at DESC) WHERE deleted_at IS NULL;

-- Add composite index for daily_analytics for date range queries
CREATE INDEX IF NOT EXISTS idx_daily_analytics_date ON daily_analytics (date DESC);

-- Add composite index for hourly_activity for hourly queries
CREATE INDEX IF NOT EXISTS idx_hourly_activity_date ON hourly_activity (date DESC);

-- Add composite index for channel_activity for room activity queries
CREATE INDEX IF NOT EXISTS idx_channel_activity_room_date ON channel_activity (room_id, date DESC);

-- Add composite index for user_status_model for online status lookups
CREATE INDEX IF NOT EXISTS idx_user_status_user ON user_status_model (user_id);

-- Add composite index for scheduled_message for pending message processing
CREATE INDEX IF NOT EXISTS idx_scheduled_messages_pending ON scheduled_messages (scheduled_at ASC) WHERE is_sent = false AND is_cancelled = false;

-- Add composite index for message_reminder for due reminder processing
CREATE INDEX IF NOT EXISTS idx_message_reminders_due ON message_reminders (remind_at ASC) WHERE is_triggered = false;

-- +migrate Down
DROP INDEX IF EXISTS idx_audit_logs_action_created;
DROP INDEX IF EXISTS idx_audit_logs_target;
DROP INDEX IF EXISTS idx_user_preferences_user_id;
DROP INDEX IF EXISTS idx_categories_sort_order;
DROP INDEX IF EXISTS idx_files_room_created;
DROP INDEX IF EXISTS idx_files_user_created;
DROP INDEX IF EXISTS idx_messages_user_created;
DROP INDEX IF EXISTS idx_invites_room_code;
DROP INDEX IF EXISTS idx_message_read_user_room;
DROP INDEX IF EXISTS idx_bans_room_user;
DROP INDEX IF EXISTS idx_reports_status_created;
DROP INDEX IF EXISTS idx_reports_reporter;
DROP INDEX IF EXISTS idx_reports_reported;
DROP INDEX IF EXISTS idx_webhooks_room_active;
DROP INDEX IF EXISTS idx_webhook_logs_webhook_created;
DROP INDEX IF EXISTS idx_sessions_user_expires;
DROP INDEX IF EXISTS idx_refresh_tokens_user;
DROP INDEX IF EXISTS idx_blocks_blocker;
DROP INDEX IF EXISTS idx_threads_parent;
DROP INDEX IF EXISTS idx_thread_messages_thread;
DROP INDEX IF EXISTS idx_polls_room_active;
DROP INDEX IF EXISTS idx_polls_user;
DROP INDEX IF EXISTS idx_poll_votes_option;
DROP INDEX IF EXISTS idx_server_emoji_created;
DROP INDEX IF EXISTS idx_server_sticker_created;
DROP INDEX IF EXISTS idx_soundboard_sounds_created;
DROP INDEX IF EXISTS idx_daily_analytics_date;
DROP INDEX IF EXISTS idx_hourly_activity_date;
DROP INDEX IF EXISTS idx_channel_activity_room_date;
DROP INDEX IF EXISTS idx_user_status_user;
DROP INDEX IF EXISTS idx_scheduled_messages_pending;
DROP INDEX IF EXISTS idx_message_reminders_due;
