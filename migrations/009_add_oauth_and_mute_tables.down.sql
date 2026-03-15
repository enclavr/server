-- +migrate Down
-- Rollback new models and indexes

DROP INDEX IF EXISTS idx_oauth_accounts_user_id;
DROP INDEX IF EXISTS idx_oauth_accounts_provider;
DROP INDEX IF EXISTS idx_oauth_accounts_user_provider;
DROP TABLE IF EXISTS oauth_accounts;

DROP INDEX IF EXISTS idx_room_mutes_user_room;
DROP INDEX IF EXISTS idx_room_mutes_room;
DROP INDEX IF EXISTS idx_room_mutes_expires;
DROP TABLE IF EXISTS room_mutes;

DROP INDEX IF EXISTS idx_messages_user_id;
DROP INDEX IF EXISTS idx_direct_messages_receiver;
DROP INDEX IF EXISTS idx_user_rooms_user_id;
DROP INDEX IF EXISTS idx_message_reactions_message;
DROP INDEX IF EXISTS idx_user_status_model_status;
DROP INDEX IF EXISTS idx_user_rooms_user_role;
DROP INDEX IF EXISTS idx_user_privacy_settings_user;
DROP INDEX IF EXISTS idx_notification_preferences_user;
