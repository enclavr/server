-- +migrate Down
-- Rollback indexes added in 013

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
