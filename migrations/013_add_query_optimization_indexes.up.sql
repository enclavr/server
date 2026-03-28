-- +migrate Up
-- Additional performance indexes and query optimizations

-- Index for audit logs by user and created_at (common filter pattern)
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_created ON audit_logs(user_id, created_at DESC);

-- Index for audit logs compound filter (action + created_at)
CREATE INDEX IF NOT EXISTS idx_audit_logs_action_created ON audit_logs(action, created_at DESC);

-- Index for user preferences by user_id (already exists but verify)
-- Note: uniqueIndex already exists on user_id

-- Index for category by sort_order (list view optimization)
CREATE INDEX IF NOT EXISTS idx_categories_sort_order ON categories(sort_order, name);

-- Index for attachment by message_id and created_at
CREATE INDEX IF NOT EXISTS idx_attachments_message_created ON attachments(message_id, created_at DESC);

-- Index for file by room and created_at (file listing)
CREATE INDEX IF NOT EXISTS idx_files_room_created ON files(room_id, created_at DESC);

-- Index for file by user and created_at
CREATE INDEX IF NOT EXISTS idx_files_user_created ON files(user_id, created_at DESC);

-- Index for scheduled messages (pending)
CREATE INDEX IF NOT EXISTS idx_scheduled_pending ON scheduled_messages(is_sent, is_cancelled, scheduled_at ASC);

-- Index for message reminders (pending)
CREATE INDEX IF NOT EXISTS idx_reminders_pending ON message_reminders(is_triggered, remind_at ASC);

-- Index for thread by parent_id
CREATE INDEX IF NOT EXISTS idx_threads_parent ON threads(parent_id);

-- Index for poll by room and created_at
CREATE INDEX IF NOT EXISTS idx_polls_room_created ON polls(room_id, created_at DESC);

-- Index for report by status
CREATE INDEX IF NOT EXISTS idx_reports_status ON reports(status, created_at DESC);

-- Index for report by reporter
CREATE INDEX IF NOT EXISTS idx_reports_reporter ON reports(reporter_id, created_at DESC);

-- Index for report by reported
CREATE INDEX IF NOT EXISTS idx_reports_reported ON reports(reported_id, created_at DESC);

-- Index for bookmark by user
CREATE INDEX IF NOT EXISTS idx_bookmarks_user ON bookmarks(user_id, created_at DESC);

-- Index for block (blocker + blocked) - common query pattern
CREATE INDEX IF NOT EXISTS idx_block_relations ON blocks(blocker_id, blocked_id);

-- Index for room template by created_by
CREATE INDEX IF NOT EXISTS idx_room_templates_creator ON room_templates(created_by, created_at DESC);

-- Index for user privacy settings by user_id (already uniqueIndex)
-- Note: uniqueIndex already exists on user_id

-- +migrate Down
-- Drop indexes
DROP INDEX IF EXISTS idx_audit_logs_user_created;
DROP INDEX IF EXISTS idx_audit_logs_action_created;
DROP INDEX IF EXISTS idx_categories_sort_order;
DROP INDEX IF EXISTS idx_attachments_message_created;
DROP INDEX IF EXISTS idx_files_room_created;
DROP INDEX IF EXISTS idx_files_user_created;
DROP INDEX IF EXISTS idx_message_read_user_room;
DROP INDEX IF EXISTS idx_user_status_user;
DROP INDEX IF EXISTS idx_scheduled_pending;
DROP INDEX IF EXISTS idx_reminders_pending;
DROP INDEX IF EXISTS idx_threads_parent;
DROP INDEX IF EXISTS idx_polls_room_created;
DROP INDEX IF EXISTS idx_reports_status;
DROP INDEX IF EXISTS idx_reports_reporter;
DROP INDEX IF EXISTS idx_reports_reported;
DROP INDEX IF EXISTS idx_bookmarks_user;
DROP INDEX IF EXISTS idx_block_relations;
DROP INDEX IF EXISTS idx_room_templates_creator;
