CREATE TABLE IF NOT EXISTS typing_indicators (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    room_id UUID,
    dm_user_id UUID,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    CONSTRAINT fk_typing_indicators_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_typing_indicators_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
    CONSTRAINT fk_typing_indicators_dm_user FOREIGN KEY (dm_user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_typing_room ON typing_indicators (room_id, user_id) WHERE room_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_typing_dm ON typing_indicators (dm_user_id, user_id) WHERE dm_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_typing_expires ON typing_indicators (expires_at);

-- Add channel_id column to voice_sessions for tracking voice channel sessions
ALTER TABLE voice_sessions ADD COLUMN IF NOT EXISTS channel_id UUID;
ALTER TABLE voice_sessions ADD COLUMN IF NOT EXISTS ended_at TIMESTAMP WITH TIME ZONE;
CREATE INDEX IF NOT EXISTS idx_voice_sessions_user ON voice_sessions (user_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_voice_sessions_room ON voice_sessions (room_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_voice_sessions_active ON voice_sessions (user_id) WHERE ended_at IS NULL;
