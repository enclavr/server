-- +migrate Down
DROP TABLE IF EXISTS room_participants CASCADE;
DROP TABLE IF EXISTS user_activities CASCADE;
DROP TABLE IF EXISTS message_edit_history CASCADE;
DROP TABLE IF EXISTS room_bookmarks CASCADE;
DROP TABLE IF EXISTS notification_preferences CASCADE;

DROP INDEX IF EXISTS idx_blocks_blocker_blocked;
DROP INDEX IF EXISTS idx_user_rooms_user_room;
DROP INDEX IF EXISTS idx_user_rooms_room_user;
DROP INDEX IF EXISTS idx_direct_messages_sender_receiver;
DROP INDEX IF EXISTS idx_direct_messages_receiver_sender;
DROP INDEX IF EXISTS idx_message_reactions_message_user;
DROP INDEX IF EXISTS idx_scheduled_messages_room_scheduled;
DROP INDEX IF EXISTS idx_message_reminders_user_remind;
