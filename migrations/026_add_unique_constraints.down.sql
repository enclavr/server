-- Remove unique constraints added in 026
DROP INDEX IF EXISTS idx_message_reactions_unique;
DROP INDEX IF EXISTS idx_user_rooms_unique;
DROP INDEX IF EXISTS idx_blocks_unique;
DROP INDEX IF EXISTS idx_oauth_accounts_unique;
DROP INDEX IF EXISTS idx_room_mutes_unique;
DROP INDEX IF EXISTS idx_user_connections_unique;
DROP INDEX IF EXISTS idx_preference_overrides_unique;
DROP INDEX IF EXISTS idx_room_ratings_unique;
DROP INDEX IF EXISTS idx_bookmarks_unique;
DROP INDEX IF EXISTS idx_message_reads_unique;
DROP INDEX IF EXISTS idx_bans_unique;
DROP INDEX IF EXISTS idx_channel_activity_unique;
DROP INDEX IF EXISTS idx_poll_votes_unique;
