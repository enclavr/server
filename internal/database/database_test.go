package database

import (
	"testing"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNew_WithSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	if database.DB == nil {
		t.Error("expected DB to be set")
	}
}

func TestDatabase_AutoMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Category{},
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
		t.Fatalf("auto migration failed: %v", err)
	}

	tables := []string{
		"users", "rooms", "categories", "user_rooms", "sessions",
		"refresh_tokens", "voice_sessions", "room_invites", "messages",
		"presences", "direct_messages", "webhooks", "webhook_logs",
		"pinned_messages", "message_reactions", "server_settings",
		"invites", "files", "push_subscriptions", "user_notification_settings",
	}

	for _, table := range tables {
		if !db.Migrator().HasTable(table) {
			t.Errorf("expected table %s to exist", table)
		}
	}
}

func TestDatabase_InsertAndQuery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}

	err = db.AutoMigrate(&models.User{})
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	_ = db.AutoMigrate(&models.User{})
	_ = db

	user := models.User{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hashedpassword",
	}

	err = database.Create(&user).Error
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	var foundUser models.User
	err = database.First(&foundUser, user.ID).Error
	if err != nil {
		t.Fatalf("failed to find user: %v", err)
	}

	if foundUser.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", foundUser.Username)
	}
}

func TestDatabase_Transaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	_ = db.AutoMigrate(&models.User{})

	err = database.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&models.User{
			Username:     "testuser",
			Email:        "test@example.com",
			PasswordHash: "password",
		}).Error
	})

	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
}

func TestDatabase_Transaction_Rollback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	_ = db.AutoMigrate(&models.User{})

	err = database.Transaction(func(tx *gorm.DB) error {
		return gorm.ErrInvalidField
	})

	if err == nil {
		t.Error("expected error from transaction")
	}

	var count int64
	database.Model(&models.User{}).Count(&count)
	if count != 0 {
		t.Errorf("expected no users after rollback, got %d", count)
	}
}

func TestDatabase_ConfigDSN(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "user",
		Password: "password",
		DBName:   "testdb",
		SSLMode:  "disable",
	}

	dsn := cfg.DSN()
	expected := "host=localhost port=5432 user=user password=password dbname=testdb sslmode=disable"
	if dsn != expected {
		t.Errorf("expected dsn %s, got %s", expected, dsn)
	}
}
