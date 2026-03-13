-- +migrate Down
DROP INDEX IF EXISTS idx_rooms_category_id;
DROP INDEX IF EXISTS idx_user_rooms_role;
DROP INDEX IF EXISTS idx_messages_is_deleted;
DROP INDEX IF EXISTS idx_direct_messages_sender_receiver;
DROP INDEX IF EXISTS idx_direct_messages_receiver_sender;
