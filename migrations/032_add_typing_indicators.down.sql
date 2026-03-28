DROP INDEX IF EXISTS idx_voice_sessions_active;
DROP INDEX IF EXISTS idx_voice_sessions_room;
DROP INDEX IF EXISTS idx_voice_sessions_user;
ALTER TABLE voice_sessions DROP COLUMN IF EXISTS channel_id;
ALTER TABLE voice_sessions DROP COLUMN IF EXISTS ended_at;
DROP INDEX IF EXISTS idx_typing_expires;
DROP INDEX IF EXISTS idx_typing_dm;
DROP INDEX IF EXISTS idx_typing_room;
DROP TABLE IF EXISTS typing_indicators;
