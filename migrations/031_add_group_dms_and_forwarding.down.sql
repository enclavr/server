-- Drop group_dm_messages table
DROP TABLE IF EXISTS group_dm_messages;

-- Drop group_dm_members table
DROP TABLE IF EXISTS group_dm_members;

-- Drop group_dms table
DROP TABLE IF EXISTS group_dms;

-- Remove forwarded_from from direct_messages
ALTER TABLE direct_messages DROP COLUMN IF EXISTS forwarded_from;

-- Remove forwarded_from from messages
ALTER TABLE messages DROP COLUMN IF EXISTS forwarded_from;

-- Remove last_message_at from user_rooms
ALTER TABLE user_rooms DROP COLUMN IF EXISTS last_message_at;
