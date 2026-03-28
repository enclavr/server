package database

import (
	"fmt"
	"os"
	"testing"

	"github.com/enclavr/server/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func GetTestDB(t *testing.T) *gorm.DB {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "enclavr")
	password := getEnv("DB_PASSWORD", "enclavr")
	dbname := getEnv("DB_NAME", "enclavr_test")
	sslmode := getEnv("DB_SSLMODE", "disable")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	return db
}

func SetupTestDB(t *testing.T) *gorm.DB {
	db := GetTestDB(t)

	err := db.Exec("CREATE TABLE IF NOT EXISTS migrations_test (id serial)").Error
	if err != nil {
		fmt.Printf("Warning: Could not create migrations table: %v\n", err)
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
		&models.VoiceChannel{},
		&models.VoiceChannelParticipant{},
	)
	if err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

func TeardownTestDB(db *gorm.DB) {
	db.Exec("DROP SCHEMA public CASCADE")
	db.Exec("CREATE SCHEMA public")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
