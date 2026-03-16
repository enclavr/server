-- +migrate Up
-- Additional performance indexes and new models

-- Create message attachment metadata table (new model for attachment extended info)
CREATE TABLE IF NOT EXISTS message_attachment_metadata (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attachment_id UUID NOT NULL,
    blur_hash VARCHAR(100),
    original_filename VARCHAR(500),
    file_extension VARCHAR(20),
    encoding VARCHAR(50),
    bit_rate INTEGER,
    sample_rate INTEGER,
    channels INTEGER,
    duration_ms INTEGER,
    width INTEGER,
    height INTEGER,
    aspect_ratio VARCHAR(20),
    color_model VARCHAR(50),
    palette_colors JSONB,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_attachment_metadata_attachment_id ON message_attachment_metadata (attachment_id);

-- Additional performance indexes for common queries

-- Index for user room listing with role
CREATE INDEX IF NOT EXISTS idx_user_rooms_role_room ON user_rooms (role, room_id);

-- Index for invite code lookups
CREATE INDEX IF NOT EXISTS idx_invite_code ON invites (code) WHERE is_revoked = false;

-- Index for invite link code lookups  
CREATE INDEX IF NOT EXISTS idx_invite_link_code ON invite_links (code) WHERE is_enabled = true;

-- Index for thread message lookups
CREATE INDEX IF NOT EXISTS idx_thread_messages_user_created ON thread_messages (user_id, created_at DESC);

-- Index for direct message conversations
CREATE INDEX IF NOT EXISTS idx_direct_messages_conversation ON direct_messages (
    LEAST(sender_id, receiver_id), 
    GREATEST(sender_id, receiver_id), 
    created_at DESC
) WHERE is_deleted = false;

-- Index for ban lookups by user and room
CREATE INDEX IF NOT EXISTS idx_bans_user_room_active ON bans (user_id, room_id) WHERE deleted_at IS NULL;

-- Index for server emoji by name
CREATE INDEX IF NOT EXISTS idx_server_emoji_name ON server_emoji (name) WHERE deleted_at IS NULL;

-- Index for sticker by name
CREATE INDEX IF NOT EXISTS idx_server_sticker_name ON server_sticker (name) WHERE deleted_at IS NULL;

-- Index for soundboard by hotkey
CREATE INDEX IF NOT EXISTS idx_soundboard_hotkey ON soundboard_sound (hotkey) WHERE deleted_at IS NULL;

-- Partial index for active scheduled messages
CREATE INDEX IF NOT EXISTS idx_scheduled_active ON scheduled_messages (scheduled_at ASC) WHERE is_sent = false AND is_cancelled = false;

-- Index for webhook by room
CREATE INDEX IF NOT EXISTS idx_webhook_room_active ON webhooks (room_id, is_active) WHERE is_active = true AND deleted_at IS NULL;

-- +migrate Down
-- Drop table
DROP INDEX IF EXISTS idx_attachment_metadata_attachment_id;
DROP TABLE IF EXISTS message_attachment_metadata;

-- Drop additional performance indexes
DROP INDEX IF EXISTS idx_user_rooms_role_room;
DROP INDEX IF EXISTS idx_invite_code;
DROP INDEX IF EXISTS idx_invite_link_code;
DROP INDEX IF EXISTS idx_thread_messages_user_created;
DROP INDEX IF EXISTS idx_direct_messages_conversation;
DROP INDEX IF EXISTS idx_bans_user_room_active;
DROP INDEX IF EXISTS idx_server_emoji_name;
DROP INDEX IF EXISTS idx_server_sticker_name;
DROP INDEX IF EXISTS idx_soundboard_hotkey;
DROP INDEX IF EXISTS idx_scheduled_active;
DROP INDEX IF EXISTS idx_webhook_room_active;
