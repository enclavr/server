-- +migrate Up
-- Create scheduled_messages table
CREATE TABLE IF NOT EXISTS scheduled_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    scheduled_at TIMESTAMP WITH TIME ZONE NOT NULL,
    is_sent BOOLEAN DEFAULT false,
    is_cancelled BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scheduled_messages_room_id ON scheduled_messages (room_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_messages_user_id ON scheduled_messages (user_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_messages_scheduled_at ON scheduled_messages (scheduled_at);
CREATE INDEX IF NOT EXISTS idx_scheduled_messages_is_sent ON scheduled_messages (is_sent) WHERE is_sent = false AND is_cancelled = false;

-- Create message_reminders table
CREATE TABLE IF NOT EXISTS message_reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    remind_at TIMESTAMP WITH TIME ZONE NOT NULL,
    is_triggered BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_message_reminders_user_id ON message_reminders (user_id);
CREATE INDEX IF NOT EXISTS idx_message_reminders_message_id ON message_reminders (message_id);
CREATE INDEX IF NOT EXISTS idx_message_reminders_remind_at ON message_reminders (remind_at) WHERE is_triggered = false;

-- Create room_templates table
CREATE TABLE IF NOT EXISTS room_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    settings JSONB DEFAULT '{}'::jsonb,
    is_public BOOLEAN DEFAULT false,
    use_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_room_templates_name ON room_templates (name);
CREATE INDEX IF NOT EXISTS idx_room_templates_created_by ON room_templates (created_by);
CREATE INDEX IF NOT EXISTS idx_room_templates_is_public ON room_templates (is_public) WHERE is_public = true;

-- Create user_privacy_settings table
CREATE TABLE IF NOT EXISTS user_privacy_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    allow_direct_messages VARCHAR(20) DEFAULT 'everyone',
    allow_room_invites VARCHAR(20) DEFAULT 'everyone',
    allow_voice_calls VARCHAR(20) DEFAULT 'everyone',
    show_online_status BOOLEAN DEFAULT true,
    show_read_receipts BOOLEAN DEFAULT true,
    show_typing_indicator BOOLEAN DEFAULT true,
    allow_search_by_email BOOLEAN DEFAULT false,
    allow_search_by_username BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_privacy_settings_user_id ON user_privacy_settings (user_id);

-- +migrate Down
DROP TABLE IF EXISTS scheduled_messages CASCADE;
DROP TABLE IF EXISTS message_reminders CASCADE;
DROP TABLE IF EXISTS room_templates CASCADE;
DROP TABLE IF EXISTS user_privacy_settings CASCADE;
