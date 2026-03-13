-- +migrate Down
DROP TABLE IF EXISTS scheduled_messages CASCADE;
DROP TABLE IF EXISTS message_reminders CASCADE;
DROP TABLE IF EXISTS room_templates CASCADE;
DROP TABLE IF EXISTS user_privacy_settings CASCADE;
