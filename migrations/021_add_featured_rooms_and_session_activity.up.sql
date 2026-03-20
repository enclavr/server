-- Migration 021: Add featured rooms and session activity tables
-- Date: 2026-03-21

-- 1. RoomFeatured table - for featured/pinned rooms on homepage
CREATE TABLE IF NOT EXISTS room_featured (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL UNIQUE REFERENCES rooms(id) ON DELETE CASCADE,
    featured_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason VARCHAR(500),
    position INT DEFAULT 0,
    starts_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_room_featured_position ON room_featured(position, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_room_featured_active ON room_featured(position) WHERE expires_at IS NULL OR expires_at > now();

-- 2. SessionActivity table - detailed session tracking for analytics
CREATE TABLE IF NOT EXISTS session_activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    room_id UUID REFERENCES rooms(id) ON DELETE SET NULL,
    activity_type VARCHAR(30) NOT NULL,
    duration INT DEFAULT 0,
    metadata JSONB,
    ip_address VARCHAR(45),
    country VARCHAR(2),
    city VARCHAR(100),
    device_type VARCHAR(20),
    browser VARCHAR(50),
    os VARCHAR(30),
    network_type VARCHAR(20),
    page_views INT DEFAULT 0,
    messages_sent INT DEFAULT 0,
    commands_run INT DEFAULT 0,
    errors_encountered INT DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_session_activities_session ON session_activities(session_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_activities_user ON session_activities(user_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_activities_room ON session_activities(room_id, started_at DESC) WHERE room_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_session_activities_type ON session_activities(activity_type, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_activities_created ON session_activities(created_at DESC);
