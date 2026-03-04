package database

import (
	"fmt"
	"testing"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNew_InvalidDSN(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Host:     "invalid-host",
		Port:     "9999",
		User:     "invalid",
		Password: "invalid",
		DBName:   "invalid",
		SSLMode:  "disable",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("expected error with invalid DSN")
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("TEST_KEY", "test_value")

	result := getEnv("TEST_KEY", "default")
	if result != "test_value" {
		t.Errorf("expected test_value, got %s", result)
	}

	result = getEnv("NON_EXISTENT", "default")
	if result != "default" {
		t.Errorf("expected default, got %s", result)
	}
}

func getTestDSN() string {
	return fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.New().String())
}

func TestNew_WithSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	if database.DB == nil {
		t.Error("expected DB to be set")
	}
}

func TestDatabase_AutoMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
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
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
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
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
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
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
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

func TestDatabase_ConfigDSN_WithSSL(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Host:     "db.example.com",
		Port:     "5432",
		User:     "admin",
		Password: "secretpass",
		DBName:   "enclavr_prod",
		SSLMode:  "require",
	}

	dsn := cfg.DSN()
	expected := "host=db.example.com port=5432 user=admin password=secretpass dbname=enclavr_prod sslmode=require"
	if dsn != expected {
		t.Errorf("expected dsn %s, got %s", expected, dsn)
	}
}

func TestDatabase_Create(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	_ = db.AutoMigrate(&models.User{})

	user := &models.User{
		Username:     "createuser",
		Email:        "create@example.com",
		PasswordHash: "hash123",
	}

	err = database.Create(user).Error
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if user.ID == uuid.Nil {
		t.Error("expected user ID to be set after create")
	}
}

func TestDatabase_First(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	_ = db.AutoMigrate(&models.User{})

	user := &models.User{
		Username:     "firstuser",
		Email:        "first@example.com",
		PasswordHash: "hash456",
	}
	database.Create(user)

	var found models.User
	err = database.First(&found, "username = ?", "firstuser").Error
	if err != nil {
		t.Fatalf("failed to find user: %v", err)
	}

	if found.Username != "firstuser" {
		t.Errorf("expected username firstuser, got %s", found.Username)
	}
}

func TestDatabase_Where(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	_ = db.AutoMigrate(&models.User{})

	users := []models.User{
		{Username: "user1", Email: "user1@example.com", PasswordHash: "hash1"},
		{Username: "user2", Email: "user2@example.com", PasswordHash: "hash2"},
		{Username: "user3", Email: "user3@example.com", PasswordHash: "hash3"},
	}
	for _, u := range users {
		database.Create(&u)
	}

	var results []models.User
	err = database.Where("username IN ?", []string{"user1", "user2"}).Find(&results).Error
	if err != nil {
		t.Fatalf("failed to query users: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 users, got %d", len(results))
	}
}

func TestDatabase_Save(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	_ = db.AutoMigrate(&models.User{})

	user := &models.User{
		Username:     "saveuser",
		Email:        "save@example.com",
		PasswordHash: "originalhash",
	}
	database.Create(user)

	user.PasswordHash = "newhash"
	err = database.Save(user).Error
	if err != nil {
		t.Fatalf("failed to save user: %v", err)
	}

	var found models.User
	database.First(&found, user.ID)
	if found.PasswordHash != "newhash" {
		t.Errorf("expected password to be newhash, got %s", found.PasswordHash)
	}
}

func TestDatabase_Delete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	_ = db.AutoMigrate(&models.User{})

	user := &models.User{
		Username:     "deleteuser",
		Email:        "delete@example.com",
		PasswordHash: "hash",
	}
	database.Create(user)

	err = database.Delete(user).Error
	if err != nil {
		t.Fatalf("failed to delete user: %v", err)
	}

	var count int64
	database.Model(&models.User{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 users after delete, got %d", count)
	}
}

func TestDatabase_Count(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	_ = db.AutoMigrate(&models.User{})

	for i := 0; i < 5; i++ {
		database.Create(&models.User{
			Username:     fmt.Sprintf("user%d", i),
			Email:        fmt.Sprintf("user%d@example.com", i),
			PasswordHash: "hash",
		})
	}

	var count int64
	err = database.Model(&models.User{}).Count(&count).Error
	if err != nil {
		t.Fatalf("failed to count users: %v", err)
	}

	if count != 5 {
		t.Errorf("expected 5 users, got %d", count)
	}
}

func TestDatabase_Updates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	_ = db.AutoMigrate(&models.User{})

	user := &models.User{
		Username:     "original",
		Email:        "original@example.com",
		PasswordHash: "hash",
	}
	database.Create(user)

	updates := map[string]interface{}{
		"username": "updated",
		"email":    "updated@example.com",
	}
	err = database.Model(user).Updates(updates).Error
	if err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	var found models.User
	database.First(&found, user.ID)
	if found.Username != "updated" {
		t.Errorf("expected username updated, got %s", found.Username)
	}
}

func TestDatabase_Find(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}
	_ = db.AutoMigrate(&models.User{})

	users := []models.User{
		{Username: "finduser1", Email: "find1@example.com", PasswordHash: "hash1"},
		{Username: "finduser2", Email: "find2@example.com", PasswordHash: "hash2"},
		{Username: "finduser3", Email: "find3@example.com", PasswordHash: "hash3"},
	}
	for _, u := range users {
		database.Create(&u)
	}

	var results []models.User
	err = database.Find(&results).Error
	if err != nil {
		t.Fatalf("failed to find users: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 users, got %d", len(results))
	}
}

func TestDatabase_Migrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}

	database := &Database{db}

	err = database.Migrate()
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	expectedTables := []string{
		"users", "rooms", "categories", "user_rooms", "sessions",
		"refresh_tokens", "voice_sessions", "room_invites", "messages",
		"presences", "direct_messages", "webhooks", "webhook_logs",
		"pinned_messages", "message_reactions", "server_settings",
		"invites", "files", "push_subscriptions", "user_notification_settings",
	}

	var missingTables []string
	for _, table := range expectedTables {
		if !db.Migrator().HasTable(table) {
			missingTables = append(missingTables, table)
		}
	}

	if len(missingTables) > 0 {
		t.Errorf("missing tables after migration: %v", missingTables)
	}
}
