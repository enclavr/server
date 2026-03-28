-- Add last_message_at to user_rooms for slow mode enforcement
ALTER TABLE user_rooms ADD COLUMN IF NOT EXISTS last_message_at TIMESTAMPTZ;

-- Add forwarded_from to messages for message forwarding
ALTER TABLE messages ADD COLUMN IF NOT EXISTS forwarded_from UUID;

-- Add forwarded_from to direct_messages for DM forwarding
ALTER TABLE direct_messages ADD COLUMN IF NOT EXISTS forwarded_from UUID;

-- Create group_dms table
CREATE TABLE IF NOT EXISTS group_dms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) DEFAULT '',
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_group_dms_created_by ON group_dms(created_by);
CREATE INDEX IF NOT EXISTS idx_group_dms_deleted_at ON group_dms(deleted_at);

-- Create group_dm_members table
CREATE TABLE IF NOT EXISTS group_dm_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_dm_id UUID NOT NULL REFERENCES group_dms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    role VARCHAR(20) DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT idx_group_dm_member UNIQUE(group_dm_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_group_dm_members_group_dm_id ON group_dm_members(group_dm_id);
CREATE INDEX IF NOT EXISTS idx_group_dm_members_user_id ON group_dm_members(user_id);

-- Create group_dm_messages table
CREATE TABLE IF NOT EXISTS group_dm_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_dm_id UUID NOT NULL REFERENCES group_dms(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id),
    content TEXT NOT NULL,
    is_edited BOOLEAN DEFAULT false,
    is_deleted BOOLEAN DEFAULT false,
    forwarded_from UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_group_dm_messages_group_dm_id ON group_dm_messages(group_dm_id);
CREATE INDEX IF NOT EXISTS idx_group_dm_messages_sender_id ON group_dm_messages(sender_id);
CREATE INDEX IF NOT EXISTS idx_group_dm_messages_created_at ON group_dm_messages(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_group_dm_messages_deleted_at ON group_dm_messages(deleted_at);
