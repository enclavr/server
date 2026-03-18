-- Migration 018: Rollback new feature tables
DROP TABLE IF EXISTS message_drafts CASCADE;
DROP TABLE IF EXISTS blocked_users CASCADE;
DROP TABLE IF EXISTS room_bookmarks CASCADE;
DROP TABLE IF EXISTS user_connections CASCADE;
