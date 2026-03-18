-- Drop foreign key constraints
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS fk_notifications_user;
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS fk_notifications_room;
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS fk_notifications_actor;

-- Drop indexes
DROP INDEX IF EXISTS idx_notifications_user_id;
DROP INDEX IF EXISTS idx_notifications_type;
DROP INDEX IF EXISTS idx_notifications_is_read;
DROP INDEX IF EXISTS idx_notifications_archived;
DROP INDEX IF EXISTS idx_notifications_room_id;
DROP INDEX IF EXISTS idx_notifications_user_unread;
DROP INDEX IF EXISTS idx_notifications_created_at;

-- Drop table
DROP TABLE IF EXISTS notifications;
