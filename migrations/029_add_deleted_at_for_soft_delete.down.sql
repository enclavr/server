-- +migrate Down
DROP INDEX IF EXISTS idx_messages_deleted_at;
DROP INDEX IF EXISTS idx_direct_messages_deleted_at;
DROP INDEX IF EXISTS idx_thread_messages_deleted_at;

ALTER TABLE messages DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE direct_messages DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE thread_messages DROP COLUMN IF EXISTS deleted_at;
