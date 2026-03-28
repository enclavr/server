-- Migration 018: Add new feature tables (drafts, blocks, bookmarks, connections)
-- Date: 2026-03-18

-- 1. MessageDrafts table - auto-save message drafts
CREATE TABLE IF NOT EXISTS message_drafts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    room_id UUID REFERENCES rooms(id) ON DELETE CASCADE,
    content TEXT NOT NULL DEFAULT '',
    is_draft BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_message_drafts_user_room ON message_drafts(user_id, room_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_message_drafts_user ON message_drafts(user_id, updated_at DESC) WHERE deleted_at IS NULL;

-- 2. BlockedUsers table - user blocking functionality
CREATE TABLE IF NOT EXISTS blocked_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    blocker_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason VARCHAR(500),
    created_at TIMESTAMPTZ DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE(blocker_id, blocked_id)
);

CREATE INDEX IF NOT EXISTS idx_blocked_users_blocker ON blocked_users(blocker_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_blocked_users_blocked ON blocked_users(blocked_id, created_at DESC) WHERE deleted_at IS NULL;

-- 3. RoomBookmarks table - bookmark favorite rooms
CREATE TABLE IF NOT EXISTS room_bookmarks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    note VARCHAR(500),
    sort_order INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE(user_id, room_id)
);

CREATE INDEX IF NOT EXISTS idx_room_bookmarks_user ON room_bookmarks(user_id, sort_order, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_room_bookmarks_room ON room_bookmarks(room_id, created_at DESC) WHERE deleted_at IS NULL;

-- 4. UserConnections table - friends/followers system
CREATE TABLE IF NOT EXISTS user_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connected_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    direction VARCHAR(20) NOT NULL DEFAULT 'oneway',
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE(user_id, connected_user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_connections_user ON user_connections(user_id, status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_user_connections_connected ON user_connections(connected_user_id, status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_user_connections_status ON user_connections(status, created_at DESC) WHERE deleted_at IS NULL;
