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
	)
	if err != nil {
		return err
	}

	d.Exec("CREATE INDEX IF NOT EXISTS idx_messages_content_fts ON messages USING gin(to_tsvector('english', content))")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_messages_room_id_created_at ON messages(room_id, created_at DESC)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_messages_user_id ON messages(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_presence_user_id_room_id ON presence(user_id, room_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_presence_updated_at ON presence(updated_at DESC)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_direct_messages_user_id ON direct_messages(user_id)")
	d.Exec("CREATE INDEX IF NOT EXISTS idx_direct_messages_recipient_id ON direct_messages(recipient_id)")
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

	log.Println("Database migration completed")
	return nil
}
