-- +migrate Up
-- Add new models and indexes

-- Create oauth_accounts table
CREATE TABLE IF NOT EXISTS oauth_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    provider VARCHAR(20) NOT NULL,
    provider_id VARCHAR(255) NOT NULL,
    access_token TEXT,
    refresh_token TEXT,
    expires_at TIMESTAMP WITH TIME ZONE,
    scope VARCHAR(500),
    avatar_url VARCHAR(500),
    profile_data JSONB,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_oauth_accounts_user_id ON oauth_accounts (user_id);
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_provider ON oauth_accounts (provider, provider_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_user_provider ON oauth_accounts (user_id, provider) WHERE deleted_at IS NULL;

-- Create room_mutes table
CREATE TABLE IF NOT EXISTS room_mutes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    room_id UUID NOT NULL,
    muted_by UUID NOT NULL,
    reason VARCHAR(500),
    expires_at TIMESTAMP WITH TIME ZONE,
    is_permanent BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_room_mutes_user_room ON room_mutes (user_id, room_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_room_mutes_room ON room_mutes (room_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_room_mutes_expires ON room_mutes (expires_at) WHERE expires_at IS NOT NULL AND deleted_at IS NULL;

-- Add missing indexes for query optimization

-- Index on messages.user_id for user message lookups
CREATE INDEX IF NOT EXISTS idx_messages_user_id ON messages (user_id);

-- Index on direct_messages for receiver lookups
CREATE INDEX IF NOT EXISTS idx_direct_messages_receiver ON direct_messages (receiver_id, created_at DESC) WHERE is_deleted = false;

-- Index on user_rooms for user room lookups
CREATE INDEX IF NOT EXISTS idx_user_rooms_user_id ON user_rooms (user_id);

-- Index on message_reactions for message reactions count
CREATE INDEX IF NOT EXISTS idx_message_reactions_message ON message_reactions (message_id, emoji);

-- Index on user_status_model for status lookups
CREATE INDEX IF NOT EXISTS idx_user_status_model_status ON user_status_model (status);

-- Composite index for user_rooms role queries
CREATE INDEX IF NOT EXISTS idx_user_rooms_user_role ON user_rooms (user_id, role);

-- Index for user privacy settings lookups
CREATE INDEX IF NOT EXISTS idx_user_privacy_settings_user ON user_privacy_settings (user_id);

-- Index for notification preferences lookups
CREATE INDEX IF NOT EXISTS idx_notification_preferences_user ON notification_preferences (user_id);

-- +migrate Down
DROP INDEX IF EXISTS idx_oauth_accounts_user_id;
DROP INDEX IF EXISTS idx_oauth_accounts_provider;
DROP INDEX IF EXISTS idx_oauth_accounts_user_provider;
DROP TABLE IF EXISTS oauth_accounts;

DROP INDEX IF EXISTS idx_room_mutes_user_room;
DROP INDEX IF EXISTS idx_room_mutes_room;
DROP INDEX IF EXISTS idx_room_mutes_expires;
DROP TABLE IF EXISTS room_mutes;

DROP INDEX IF EXISTS idx_messages_user_id;
DROP INDEX IF EXISTS idx_direct_messages_receiver;
DROP INDEX IF EXISTS idx_user_rooms_user_id;
DROP INDEX IF EXISTS idx_message_reactions_message;
DROP INDEX IF EXISTS idx_user_status_model_status;
DROP INDEX IF EXISTS idx_user_rooms_user_role;
DROP INDEX IF EXISTS idx_user_privacy_settings_user;
DROP INDEX IF EXISTS idx_notification_preferences_user;
