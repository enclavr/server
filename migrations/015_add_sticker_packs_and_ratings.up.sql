-- Migration 015: Add sticker packs, room ratings, activity logs, and room metrics
-- Created: 2026-03-18

-- Create sticker_packs table
CREATE TABLE IF NOT EXISTS sticker_packs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    cover_url VARCHAR(500),
    is_premium BOOLEAN DEFAULT FALSE,
    price INTEGER DEFAULT 0,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    is_global BOOLEAN DEFAULT FALSE,
    use_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_sticker_packs_created_by ON sticker_packs(created_by);
CREATE INDEX IF NOT EXISTS idx_sticker_packs_is_global ON sticker_packs(is_global) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sticker_packs_is_premium ON sticker_packs(is_premium) WHERE deleted_at IS NULL;

-- Create room_ratings table
CREATE TABLE IF NOT EXISTS room_ratings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
    comment TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(room_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_room_ratings_room_id ON room_ratings(room_id);
CREATE INDEX IF NOT EXISTS idx_room_ratings_user_id ON room_ratings(user_id);
CREATE INDEX IF NOT EXISTS idx_room_ratings_rating ON room_ratings(rating) WHERE deleted_at IS NULL;

-- Create user_activity_logs table
CREATE TABLE IF NOT EXISTS user_activity_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_type VARCHAR(50) NOT NULL,
    room_id UUID REFERENCES rooms(id) ON DELETE SET NULL,
    target_type VARCHAR(50),
    target_id UUID,
    metadata JSONB,
    ip_address VARCHAR(45),
    user_agent VARCHAR(500),
    session_id UUID REFERENCES sessions(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_activity_logs_user_id ON user_activity_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_user_activity_logs_activity_type ON user_activity_logs(activity_type);
CREATE INDEX IF NOT EXISTS idx_user_activity_logs_room_id ON user_activity_logs(room_id);
CREATE INDEX IF NOT EXISTS idx_user_activity_logs_created_at ON user_activity_logs(created_at DESC);

-- Create room_metrics table
CREATE TABLE IF NOT EXISTS room_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    message_count INTEGER DEFAULT 0,
    unique_users INTEGER DEFAULT 0,
    voice_minutes INTEGER DEFAULT 0,
    file_uploads INTEGER DEFAULT 0,
    avg_response_time_ms INTEGER,
    peak_users INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(room_id, date)
);

CREATE INDEX IF NOT EXISTS idx_room_metrics_room_id ON room_metrics(room_id);
CREATE INDEX IF NOT EXISTS idx_room_metrics_date ON room_metrics(date DESC);
CREATE INDEX IF NOT EXISTS idx_room_metrics_room_date ON room_metrics(room_id, date DESC);
