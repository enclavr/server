-- +migrate Down
-- Rollback migration 011

-- Drop message attachment metadata table
DROP INDEX IF EXISTS idx_attachment_metadata_attachment_id;
DROP TABLE IF EXISTS message_attachment_metadata;

-- Drop additional performance indexes
DROP INDEX IF EXISTS idx_user_rooms_role_room;
DROP INDEX IF EXISTS idx_invite_code;
DROP INDEX IF EXISTS idx_invite_link_code;
DROP INDEX IF EXISTS idx_thread_messages_user_created;
DROP INDEX IF EXISTS idx_direct_messages_conversation;
DROP INDEX IF EXISTS idx_bans_user_room_active;
DROP INDEX IF EXISTS idx_server_emoji_name;
DROP INDEX IF EXISTS idx_server_sticker_name;
DROP INDEX IF EXISTS idx_soundboard_hotkey;
DROP INDEX IF EXISTS idx_scheduled_active;
DROP INDEX IF EXISTS idx_webhook_room_active;
