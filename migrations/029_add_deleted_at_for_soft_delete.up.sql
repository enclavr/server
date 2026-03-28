-- +migrate Up
-- Add deleted_at columns for consistent GORM soft-delete support
-- Models previously used IsDeleted boolean without GORM auto-scoping

-- Add deleted_at to messages table
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'messages' AND column_name = 'deleted_at') THEN
        ALTER TABLE messages ADD COLUMN deleted_at TIMESTAMPTZ;
        CREATE INDEX IF NOT EXISTS idx_messages_deleted_at ON messages(deleted_at);
    END IF;
END $$;

-- Add deleted_at to direct_messages table
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'direct_messages' AND column_name = 'deleted_at') THEN
        ALTER TABLE direct_messages ADD COLUMN deleted_at TIMESTAMPTZ;
        CREATE INDEX IF NOT EXISTS idx_direct_messages_deleted_at ON direct_messages(deleted_at);
    END IF;
END $$;

-- Add deleted_at to thread_messages table
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'thread_messages' AND column_name = 'deleted_at') THEN
        ALTER TABLE thread_messages ADD COLUMN deleted_at TIMESTAMPTZ;
        CREATE INDEX IF NOT EXISTS idx_thread_messages_deleted_at ON thread_messages(deleted_at);
    END IF;
END $$;

-- Sync existing soft-deleted records: set deleted_at where is_deleted = true
UPDATE messages SET deleted_at = updated_at WHERE is_deleted = true AND deleted_at IS NULL;
UPDATE direct_messages SET deleted_at = updated_at WHERE is_deleted = true AND deleted_at IS NULL;
UPDATE thread_messages SET deleted_at = updated_at WHERE is_deleted = true AND deleted_at IS NULL;

-- +migrate Down
DROP INDEX IF EXISTS idx_messages_deleted_at;
DROP INDEX IF EXISTS idx_direct_messages_deleted_at;
DROP INDEX IF EXISTS idx_thread_messages_deleted_at;

ALTER TABLE messages DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE direct_messages DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE thread_messages DROP COLUMN IF EXISTS deleted_at;
