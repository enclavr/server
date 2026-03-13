-- +migrate Up
-- Add missing indexes for better query performance

-- Index for rooms by category (for category-based room queries)
CREATE INDEX IF NOT EXISTS idx_rooms_category_id ON rooms (category_id) WHERE category_id IS NOT NULL;

-- Index for user_rooms by role (for role-based queries)
CREATE INDEX IF NOT EXISTS idx_user_rooms_role ON user_rooms (role);

-- Partial index for active messages (excluding deleted)
CREATE INDEX IF NOT EXISTS idx_messages_is_deleted ON messages (is_deleted) WHERE is_deleted = false;

-- Composite indexes for direct messages (efficient DM queries)
CREATE INDEX IF NOT EXISTS idx_direct_messages_sender_receiver ON direct_messages (sender_id, receiver_id);
CREATE INDEX IF NOT EXISTS idx_direct_messages_receiver_sender ON direct_messages (receiver_id, sender_id);

-- +migrate Down
DROP INDEX IF EXISTS idx_rooms_category_id;
DROP INDEX IF EXISTS idx_user_rooms_role;
DROP INDEX IF EXISTS idx_messages_is_deleted;
DROP INDEX IF EXISTS idx_direct_messages_sender_receiver;
DROP INDEX IF EXISTS idx_direct_messages_receiver_sender;
