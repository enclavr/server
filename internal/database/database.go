package database

import (
	"log"
	"strings"
	"time"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Database struct {
	*gorm.DB
}

func New(cfg *config.DatabaseConfig) (*Database, error) {
	dsn := cfg.DSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	db.Exec("SET statement_timeout = '10s'")
	db.Exec("SET lock_timeout = '5s'")

	log.Println("Database connected successfully")
	return &Database{db}, nil
}

func (d *Database) Migrate() error {
	isPostgres := isPostgresDB(d)

	if isPostgres {
		d.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"")
		d.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm")
		d.Exec("CREATE EXTENSION IF NOT EXISTS pg_stat_statements")
	}

	err := d.AutoMigrate(
		&models.Category{},
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Session{},
		&models.RefreshToken{},
		&models.VoiceSession{},
		&models.RoomInvite{},
		&models.Message{},
		&models.Presence{},
		&models.DirectMessage{},
		&models.Webhook{},
		&models.WebhookLog{},
		&models.PinnedMessage{},
		&models.MessageReaction{},
		&models.ServerSettings{},
		&models.Invite{},
		&models.File{},
		&models.PushSubscription{},
		&models.UserNotificationSettings{},
		&models.Thread{},
		&models.ThreadMessage{},
		&models.Poll{},
		&models.PollOption{},
		&models.PollVote{},
		&models.ServerEmoji{},
		&models.ServerSticker{},
		&models.SoundboardSound{},
		&models.DailyAnalytics{},
		&models.HourlyActivity{},
		&models.ChannelActivity{},
		&models.AuditLog{},
		&models.Ban{},
		&models.Report{},
		&models.Bookmark{},
		&models.UserPreferences{},
		&models.RoomSettings{},
		&models.Block{},
		&models.MessageRead{},
		&models.Attachment{},
		&models.CategoryPermission{},
		&models.UserDevice{},
		&models.AuditLogExclusion{},
		&models.APIKey{},
		&models.Role{},
		&models.RolePermission{},
		&models.UserRole{},
		&models.UserNotification{},
	)
	if err != nil {
		return err
	}

	d.Exec("CREATE INDEX IF NOT EXISTS idx_messages_room_id_created_at ON messages(room_id, created_at DESC)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_messages_user_id ON messages(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_presence_user_id ON presences(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_presence_room_id ON presences(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_presence_last_seen ON presences(last_seen DESC)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_direct_messages_sender_id ON direct_messages(sender_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_direct_messages_receiver_id ON direct_messages(receiver_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_user_rooms_user_id ON user_rooms(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_user_rooms_room_id ON user_rooms(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_invites_code ON invites(code)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_invites_room_id ON invites(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_voice_sessions_room_id ON voice_sessions(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_pinned_messages_room_id ON pinned_messages(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_message_reactions_message_id ON message_reactions(message_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_files_room_id ON files(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_webhooks_room_id ON webhooks(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_threads_room_id ON threads(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_threads_parent_id ON threads(parent_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_thread_messages_thread_id ON thread_messages(thread_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_polls_room_id ON polls(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_poll_options_poll_id ON poll_options(poll_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_poll_votes_poll_id ON poll_votes(poll_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_poll_votes_user_id ON poll_votes(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_reports_reporter_id ON reports(reporter_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_reports_reported_id ON reports(reported_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_reports_status ON reports(status)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_bans_user_id ON bans(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_bans_room_id ON bans(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_bookmarks_user_id ON bookmarks(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_server_emoji_created_by ON server_emoji(created_by)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_server_stickers_created_by ON server_stickers(created_by)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_soundboard_sounds_created_by ON soundboard_sounds(created_by)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_channel_activity_room_id_date ON channel_activity(room_id, date DESC)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_user_preferences_user_id ON user_preferences(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_room_settings_room_id ON room_settings(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_blocks_blocker_blocked ON blocks(blocker_id, blocked_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_push_subscriptions_endpoint_user ON push_subscriptions(endpoint, user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_voice_sessions_user_id ON voice_sessions(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_room_invites_room_id ON room_invites(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_invite_links_code ON invite_links(code)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_user_rooms_user_room_role ON user_rooms(user_id, room_id, role)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_messages_is_deleted ON messages(is_deleted)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_direct_messages_is_deleted ON direct_messages(is_deleted)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_thread_messages_is_deleted ON thread_messages(is_deleted)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_attachments_message_id ON attachments(message_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_attachments_file_id ON attachments(file_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_category_permissions_category_id ON category_permissions(category_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_category_permissions_user_id ON category_permissions(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_user_devices_user_id ON user_devices(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_user_devices_device_id ON user_devices(device_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_audit_log_exclusions_user_id ON audit_log_exclusions(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_audit_log_exclusions_action ON audit_log_exclusions(action)")

	d.Exec("CREATE INDEX IF NOT EXISTS idx_users_lower_username ON users(LOWER(username))")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_messages_room_user ON messages(room_id, user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at DESC)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_direct_messages_users ON direct_messages(sender_id, receiver_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_poll_votes_unique ON poll_votes(poll_id, user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_reports_room_status ON reports(room_id, status)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_presence_user_room ON presences(user_id, room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_files_user_room ON files(user_id, room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_bookmarks_message_user ON bookmarks(message_id, user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_threads_parent_created ON threads(parent_id, created_at DESC)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_user_status_user ON user_statuses(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_category_permissions_user_role ON category_permissions(user_id, role_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_attachments_user ON attachments(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_invites_created_expires ON invites(created_at, expires_at)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_webhooks_active_room ON webhooks(room_id, is_active)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_notifications_user_read ON user_notifications(user_id, is_read)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_analytics_date ON daily_analytics(date DESC)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_hourly_activity_date_hour ON hourly_activity(date, hour)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_api_keys_active ON api_keys(is_active)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_roles_room_id ON roles(room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_role_permissions_role_id ON role_permissions(role_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles(role_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_notifications_user_type ON user_notifications(user_id, type)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_notifications_created ON user_notifications(created_at DESC)")

	log.Println("Database migration completed")
	return nil
}

func isPostgresDB(db *Database) bool {
	var result string
	db.Raw("SELECT version()").Scan(&result)
	return strings.Contains(result, "PostgreSQL")
}
