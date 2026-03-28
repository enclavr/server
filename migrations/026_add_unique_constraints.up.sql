-- Add unique constraints to prevent duplicate records
-- Issue #87: Missing unique constraints allow duplicate records across 15+ tables

-- MessageReaction: prevent duplicate reactions
CREATE UNIQUE INDEX IF NOT EXISTS idx_message_reactions_unique
    ON message_reactions (message_id, user_id, emoji);

-- UserRoom: prevent duplicate room memberships
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_rooms_unique
    ON user_rooms (user_id, room_id);

-- Block: prevent duplicate blocks
CREATE UNIQUE INDEX IF NOT EXISTS idx_blocks_unique
    ON blocks (blocker_id, blocked_id);

-- OAuthAccount: prevent duplicate OAuth links
CREATE UNIQUE INDEX IF NOT EXISTS idx_oauth_accounts_unique
    ON oauth_accounts (provider, provider_id);

-- RoomMute: prevent duplicate mutes
CREATE UNIQUE INDEX IF NOT EXISTS idx_room_mutes_unique
    ON room_mutes (user_id, room_id);

-- UserConnection: prevent duplicate connections
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_connections_unique
    ON user_connections (user_id, connected_user_id);

-- PreferenceOverride: prevent duplicate overrides
CREATE UNIQUE INDEX IF NOT EXISTS idx_preference_overrides_unique
    ON preference_overrides (user_id, room_id, setting_key);

-- RoomRating: prevent duplicate ratings
CREATE UNIQUE INDEX IF NOT EXISTS idx_room_ratings_unique
    ON room_ratings (room_id, user_id);

-- Bookmark: prevent duplicate bookmarks
CREATE UNIQUE INDEX IF NOT EXISTS idx_bookmarks_unique
    ON bookmarks (user_id, message_id);

-- MessageRead: prevent duplicate read records
CREATE UNIQUE INDEX IF NOT EXISTS idx_message_reads_unique
    ON message_reads (user_id, room_id, message_id);

-- Ban: prevent duplicate bans
CREATE UNIQUE INDEX IF NOT EXISTS idx_bans_unique
    ON bans (user_id, room_id);

-- ChannelActivity: prevent duplicate activity records
CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_activity_unique
    ON channel_activities (room_id, date);

-- PollVote: prevent duplicate votes per user per poll
CREATE UNIQUE INDEX IF NOT EXISTS idx_poll_votes_unique
    ON poll_votes (poll_id, user_id);
