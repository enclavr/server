-- +migrate Up
-- Create voice_channels table for persistent voice channels within rooms
CREATE TABLE IF NOT EXISTS voice_channels (
    id UUID PRIMARY KEY,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500) DEFAULT '',
    max_participants INTEGER DEFAULT 10,
    is_private BOOLEAN DEFAULT false,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_voice_channels_room_id ON voice_channels(room_id);
CREATE INDEX IF NOT EXISTS idx_voice_channels_deleted_at ON voice_channels(deleted_at);
CREATE INDEX IF NOT EXISTS idx_voice_channels_created_by ON voice_channels(created_by);

-- Create voice_channel_participants table for tracking who is in each voice channel
CREATE TABLE IF NOT EXISTS voice_channel_participants (
    id UUID PRIMARY KEY,
    channel_id UUID NOT NULL REFERENCES voice_channels(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    is_muted BOOLEAN DEFAULT false,
    is_deafened BOOLEAN DEFAULT false,
    is_speaking BOOLEAN DEFAULT false,
    joined_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(channel_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_voice_channel_participants_channel_id ON voice_channel_participants(channel_id);
CREATE INDEX IF NOT EXISTS idx_voice_channel_participants_user_id ON voice_channel_participants(user_id);

-- +migrate Down
DROP TABLE IF EXISTS voice_channel_participants;
DROP TABLE IF EXISTS voice_channels;
