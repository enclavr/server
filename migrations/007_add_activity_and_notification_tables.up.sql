-- +migrate Up
-- Create notification_preferences table for granular notification settings
CREATE TABLE IF NOT EXISTS notification_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    room_notifications JSONB DEFAULT '{}'::jsonb,
    dm_notifications VARCHAR(20) DEFAULT 'all',
    group_notifications VARCHAR(20) DEFAULT 'all',
    mention_notifications BOOLEAN DEFAULT true,
    reply_notifications BOOLEAN DEFAULT true,
    reaction_notifications BOOLEAN DEFAULT true,
    direct_message_notifications BOOLEAN DEFAULT true,
    room_invite_notifications BOOLEAN DEFAULT true,
    sound_enabled BOOLEAN DEFAULT true,
    desktop_notifications BOOLEAN DEFAULT true,
    mobile_push_enabled BOOLEAN DEFAULT true,
    quiet_hours_enabled BOOLEAN DEFAULT false,
    quiet_hours_start VARCHAR(5) DEFAULT '22:00',
    quiet_hours_end VARCHAR(5) DEFAULT '08:00',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notification_preferences_user_id ON notification_preferences (user_id);

-- Create room_bookmarks table for bookmarking rooms
CREATE TABLE IF NOT EXISTS room_bookmarks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    note VARCHAR(500),
    position INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_room_bookmarks_user_id ON room_bookmarks (user_id);
CREATE INDEX IF NOT EXISTS idx_room_bookmarks_room_id ON room_bookmarks (room_id);
CREATE INDEX IF NOT EXISTS idx_room_bookmarks_user_room ON room_bookmarks (user_id, room_id) UNIQUE;
CREATE INDEX IF NOT EXISTS idx_room_bookmarks_deleted_at ON room_bookmarks (deleted_at) WHERE deleted_at IS NULL;

-- Create message_edit_history table for tracking message edits
CREATE TABLE IF NOT EXISTS message_edit_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    old_content TEXT NOT NULL,
    new_content TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_message_edit_history_message_id ON message_edit_history (message_id);
CREATE INDEX IF NOT EXISTS idx_message_edit_history_user_id ON message_edit_history (user_id);
CREATE INDEX IF NOT EXISTS idx_message_edit_history_created_at ON message_edit_history (created_at DESC);

-- Create user_activities table for tracking user activity history
CREATE TABLE IF NOT EXISTS user_activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity VARCHAR(30) NOT NULL,
    room_id UUID REFERENCES rooms(id) ON DELETE SET NULL,
    target_id UUID,
    metadata JSONB DEFAULT '{}'::jsonb,
    ip_address VARCHAR(45),
    user_agent VARCHAR(500),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_activities_user_id ON user_activities (user_id);
CREATE INDEX IF NOT EXISTS idx_user_activities_activity ON user_activities (activity);
CREATE INDEX IF NOT EXISTS idx_user_activities_room_id ON user_activities (room_id);
CREATE INDEX IF NOT EXISTS idx_user_activities_created_at ON user_activities (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_activities_user_activity ON user_activities (user_id, activity, created_at DESC);

-- Create room_participants table for efficient room membership queries
CREATE TABLE IF NOT EXISTS room_participants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_read_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(room_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_room_participants_room_id ON room_participants (room_id);
CREATE INDEX IF NOT EXISTS idx_room_participants_user_id ON room_participants (user_id);
CREATE INDEX IF NOT EXISTS idx_room_participants_deleted_at ON room_participants (deleted_at) WHERE deleted_at IS NULL;

-- Performance indexes: Add missing composite indexes for common query patterns
-- Block table composite index
CREATE INDEX IF NOT EXISTS idx_blocks_blocker_blocked ON blocks (blocker_id, blocked_id);

-- UserRoom composite indexes for common queries
CREATE INDEX IF NOT EXISTS idx_user_rooms_user_room ON user_rooms (user_id, room_id);
CREATE INDEX IF NOT EXISTS idx_user_rooms_room_user ON user_rooms (room_id, user_id);

-- Direct messages composite index for sender/receiver queries
CREATE INDEX IF NOT EXISTS idx_direct_messages_sender_receiver ON direct_messages (sender_id, receiver_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_direct_messages_receiver_sender ON direct_messages (receiver_id, sender_id, created_at DESC);

-- Message reactions - add composite for message+user
CREATE INDEX IF NOT EXISTS idx_message_reactions_message_user ON message_reactions (message_id, user_id);

-- Scheduled messages composite index
CREATE INDEX IF NOT EXISTS idx_scheduled_messages_room_scheduled ON scheduled_messages (room_id, scheduled_at ASC) WHERE is_sent = false AND is_cancelled = false;

-- Message reminders composite index
CREATE INDEX IF NOT EXISTS idx_message_reminders_user_remind ON message_reminders (user_id, remind_at ASC) WHERE is_triggered = false;

-- +migrate Down
DROP TABLE IF EXISTS room_participants CASCADE;
DROP TABLE IF EXISTS user_activities CASCADE;
DROP TABLE IF EXISTS message_edit_history CASCADE;
DROP TABLE IF EXISTS room_bookmarks CASCADE;
DROP TABLE IF EXISTS notification_preferences CASCADE;

DROP INDEX IF EXISTS idx_blocks_blocker_blocked;
DROP INDEX IF EXISTS idx_user_rooms_user_room;
DROP INDEX IF EXISTS idx_user_rooms_room_user;
DROP INDEX IF EXISTS idx_direct_messages_sender_receiver;
DROP INDEX IF EXISTS idx_direct_messages_receiver_sender;
DROP INDEX IF EXISTS idx_message_reactions_message_user;
DROP INDEX IF EXISTS idx_scheduled_messages_room_scheduled;
DROP INDEX IF EXISTS idx_message_reminders_user_remind;
