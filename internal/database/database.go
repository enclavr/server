package database

import (
	"log"
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
	d.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"")
	d.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm")

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
	)
	if err != nil {
		return err
	}

	d.Exec("CREATE INDEX IF NOT EXISTS idx_messages_content_fts ON messages USING gin(to_tsvector('english', content))")
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

	log.Println("Database migration completed")
	return nil
}
