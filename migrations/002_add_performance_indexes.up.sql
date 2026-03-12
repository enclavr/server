-- +migrate Up
-- Performance indexes for common queries
CREATE INDEX IF NOT EXISTS idx_users_lower_username ON users(LOWER(username));
CREATE INDEX IF NOT EXISTS idx_messages_room_user ON messages(room_id, user_id);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_direct_messages_users ON direct_messages(sender_id, receiver_id);
CREATE INDEX IF NOT EXISTS idx_poll_votes_unique ON poll_votes(poll_id, user_id);
CREATE INDEX IF NOT EXISTS idx_reports_room_status ON reports(room_id, status);
CREATE INDEX IF NOT EXISTS idx_presence_user_room ON presences(user_id, room_id);
CREATE INDEX IF NOT EXISTS idx_files_user_room ON files(user_id, room_id);
CREATE INDEX IF NOT EXISTS idx_bookmarks_message_user ON bookmarks(message_id, user_id);
CREATE INDEX IF NOT EXISTS idx_threads_parent_created ON threads(parent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_category_permissions_user_role ON category_permissions(user_id, role_id);
CREATE INDEX IF NOT EXISTS idx_attachments_user ON attachments(user_id);
CREATE INDEX IF NOT EXISTS idx_invites_created_expires ON invites(created_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_webhooks_active_room ON webhooks(room_id, is_active);
CREATE INDEX IF NOT EXISTS idx_analytics_date ON daily_analytics(date DESC);
CREATE INDEX IF NOT EXISTS idx_hourly_activity_date_hour ON hourly_activity(date, hour);

-- Indexes for new models
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_active ON api_keys(is_active);
CREATE INDEX IF NOT EXISTS idx_roles_room_id ON roles(room_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_role_id ON role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles(role_id);
CREATE INDEX IF NOT EXISTS idx_notifications_user_type ON user_notifications(user_id, type);
CREATE INDEX IF NOT EXISTS idx_notifications_created ON user_notifications(created_at DESC);

-- +migrate Down
DROP INDEX IF EXISTS idx_users_lower_username;
DROP INDEX IF EXISTS idx_messages_room_user;
DROP INDEX IF EXISTS idx_messages_created_at;
DROP INDEX IF EXISTS idx_direct_messages_users;
DROP INDEX IF EXISTS idx_poll_votes_unique;
DROP INDEX IF EXISTS idx_reports_room_status;
DROP INDEX IF EXISTS idx_presence_user_room;
DROP INDEX IF EXISTS idx_files_user_room;
DROP INDEX IF EXISTS idx_bookmarks_message_user;
DROP INDEX IF EXISTS idx_threads_parent_created;
DROP INDEX IF EXISTS idx_category_permissions_user_role;
DROP INDEX IF EXISTS idx_attachments_user;
DROP INDEX IF EXISTS idx_invites_created_expires;
DROP INDEX IF EXISTS idx_webhooks_active_room;
DROP INDEX IF EXISTS idx_analytics_date;
DROP INDEX IF EXISTS idx_hourly_activity_date_hour;
DROP INDEX IF EXISTS idx_api_keys_user_id;
DROP INDEX IF EXISTS idx_api_keys_active;
DROP INDEX IF EXISTS idx_roles_room_id;
DROP INDEX IF EXISTS idx_role_permissions_role_id;
DROP INDEX IF EXISTS idx_user_roles_user_id;
DROP INDEX IF EXISTS idx_user_roles_role_id;
DROP INDEX IF EXISTS idx_notifications_user_type;
DROP INDEX IF EXISTS idx_notifications_created;
