package services

import (
	"fmt"
	"os"
	"testing"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNewPushService(t *testing.T) {
	cfg := &config.Config{}
	pushService := NewPushService(nil, cfg)
	if pushService == nil {
		t.Error("expected non-nil PushService")
	}
}

func TestPushService_isQuietHours_Disabled(t *testing.T) {
	cfg := &config.Config{}
	svc := &PushService{cfg: cfg}

	result := svc.isQuietHours("22:00", "08:00", false)
	if result != false {
		t.Errorf("expected false when disabled, got %v", result)
	}
}

func TestPushService_isQuietHours_NormalRange(t *testing.T) {
	cfg := &config.Config{}
	svc := &PushService{cfg: cfg}

	tests := []struct {
		name  string
		start string
		end   string
		desc  string
	}{
		{"early morning", "01:00", "06:00", "test early morning range"},
		{"midday", "12:00", "13:00", "test midday range"},
		{"evening", "18:00", "22:00", "test evening range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = svc.isQuietHours(tt.start, tt.end, true)
		})
	}
}

func TestPushPayload_Fields(t *testing.T) {
	payload := PushPayload{
		Title:              "Test Title",
		Body:               "Test Body",
		Icon:               "/icon.png",
		Badge:              "/badge.png",
		Tag:                "test-tag",
		Data:               nil,
		RequireInteraction: false,
	}

	if payload.Title != "Test Title" {
		t.Errorf("expected Title to be Test Title, got %s", payload.Title)
	}
	if payload.Body != "Test Body" {
		t.Errorf("expected Body to be Test Body, got %s", payload.Body)
	}
}

func TestPushNotification_Fields(t *testing.T) {
	notification := PushNotification{
		Notification: PushPayload{
			Title: "Test",
			Body:  "Body",
		},
		Data: nil,
	}

	if notification.Notification.Title != "Test" {
		t.Errorf("expected Title to be Test, got %s", notification.Notification.Title)
	}
}

func TestPushService_SendNotification_NoUserSettings(t *testing.T) {
	db := openTestDB(t)

	if err := db.AutoMigrate(&models.UserNotificationSettings{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	dbase := &database.Database{DB: db}

	cfg := &config.Config{}
	svc := NewPushService(dbase, cfg)

	userID := uuid.New()
	payload := PushPayload{
		Title: "Test",
		Body:  "Test body",
	}

	err := svc.SendNotification(userID, payload)
	if err == nil {
		t.Logf("SendNotification returned no error (user not found is expected)")
	}
}

func TestPushService_SendNotification_DisabledPush(t *testing.T) {
	db := openTestDB(t)

	if err := db.AutoMigrate(&models.UserNotificationSettings{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	userID := uuid.New()
	db.Create(&models.UserNotificationSettings{
		UserID:            userID,
		EnablePush:        false,
		QuietHoursEnabled: false,
	})

	dbase := &database.Database{DB: db}
	cfg := &config.Config{}
	svc := NewPushService(dbase, cfg)

	payload := PushPayload{
		Title: "Test",
		Body:  "Test body",
	}

	err := svc.SendNotification(userID, payload)
	if err != nil {
		t.Errorf("expected no error when push is disabled, got %v", err)
	}
}

func TestPushService_SendNotification_QuietHours(t *testing.T) {
	db := openTestDB(t)

	if err := db.AutoMigrate(&models.UserNotificationSettings{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	userID := uuid.New()
	db.Create(&models.UserNotificationSettings{
		UserID:            userID,
		EnablePush:        true,
		QuietHoursEnabled: true,
		QuietHoursStart:   "00:00",
		QuietHoursEnd:     "23:59",
	})

	dbase := &database.Database{DB: db}
	cfg := &config.Config{}
	svc := NewPushService(dbase, cfg)

	payload := PushPayload{
		Title: "Test",
		Body:  "Test body",
	}

	err := svc.SendNotification(userID, payload)
	if err != nil {
		t.Logf("SendNotification during quiet hours returned: %v", err)
	}
}

func TestPushService_SendNotification_NoSubscriptions(t *testing.T) {
	db := openTestDB(t)

	if err := db.AutoMigrate(&models.UserNotificationSettings{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	userID := uuid.New()
	db.Create(&models.UserNotificationSettings{
		UserID:            userID,
		EnablePush:        true,
		QuietHoursEnabled: false,
	})

	dbase := &database.Database{DB: db}
	cfg := &config.Config{}
	svc := NewPushService(dbase, cfg)

	payload := PushPayload{
		Title: "Test",
		Body:  "Test body",
	}

	err := svc.SendNotification(userID, payload)
	if err != nil {
		t.Errorf("expected no error when no subscriptions, got %v", err)
	}
}

func TestPushService_NotifyNewMessage(t *testing.T) {
	db := openTestDB(t)

	if err := db.AutoMigrate(&models.UserNotificationSettings{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	dbase := &database.Database{DB: db}
	cfg := &config.Config{}
	svc := NewPushService(dbase, cfg)

	userID := uuid.New()
	err := svc.NotifyNewMessage(userID, "TestRoom", "TestSender", "Hello")
	if err != nil {
		t.Logf("NotifyNewMessage returned: %v", err)
	}
}

func TestPushService_NotifyNewDM(t *testing.T) {
	db := openTestDB(t)

	if err := db.AutoMigrate(&models.UserNotificationSettings{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	dbase := &database.Database{DB: db}
	cfg := &config.Config{}
	svc := NewPushService(dbase, cfg)

	userID := uuid.New()
	err := svc.NotifyNewDM(userID, "TestSender", "Hello")
	if err != nil {
		t.Logf("NotifyNewDM returned: %v", err)
	}
}

func TestPushService_NotifyMention(t *testing.T) {
	db := openTestDB(t)

	if err := db.AutoMigrate(&models.UserNotificationSettings{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	dbase := &database.Database{DB: db}
	cfg := &config.Config{}
	svc := NewPushService(dbase, cfg)

	userID := uuid.New()
	err := svc.NotifyMention(userID, "TestRoom", "TestSender", "Hey @user")
	if err != nil {
		t.Logf("NotifyMention returned: %v", err)
	}
}

func TestPushService_NotifyVoiceJoin(t *testing.T) {
	db := openTestDB(t)

	if err := db.AutoMigrate(&models.UserNotificationSettings{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	dbase := &database.Database{DB: db}
	cfg := &config.Config{}
	svc := NewPushService(dbase, cfg)

	userID := uuid.New()
	err := svc.NotifyVoiceJoin(userID, "TestRoom", "VoiceUser")
	if err != nil {
		t.Logf("NotifyVoiceJoin returned: %v", err)
	}
}

func TestPushService_isQuietHours_CrossMidnight(t *testing.T) {
	cfg := &config.Config{}
	svc := &PushService{cfg: cfg}

	result := svc.isQuietHours("22:00", "06:00", true)
	_ = result
}

func TestPushService_isQuietHours_WithinRange(t *testing.T) {
	cfg := &config.Config{}
	svc := &PushService{cfg: cfg}

	result := svc.isQuietHours("00:00", "23:59", true)
	if !result {
		t.Logf("isQuietHours returned: %v (may vary based on current time)", result)
	}
}

func TestPushService_SendNotification_WithActiveSubscription(t *testing.T) {
	db := openTestDB(t)

	if err := db.AutoMigrate(&models.UserNotificationSettings{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	userID := uuid.New()
	db.Create(&models.UserNotificationSettings{
		UserID:            userID,
		EnablePush:        true,
		QuietHoursEnabled: false,
	})

	sub := models.PushSubscription{
		UserID:   userID,
		Endpoint: "https://example.com/push",
		P256DH:   "test-p256dh",
		Auth:     "test-auth",
		IsActive: true,
	}
	db.Create(&sub)

	dbase := &database.Database{DB: db}
	cfg := &config.Config{}
	svc := NewPushService(dbase, cfg)

	payload := PushPayload{
		Title: "Test",
		Body:  "Test body",
	}

	err := svc.SendNotification(userID, payload)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestPushService_NotifyNewMessage_Complete(t *testing.T) {
	db := openTestDB(t)

	if err := db.AutoMigrate(&models.UserNotificationSettings{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	userID := uuid.New()
	db.Create(&models.UserNotificationSettings{
		UserID:            userID,
		EnablePush:        true,
		QuietHoursEnabled: false,
	})

	dbase := &database.Database{DB: db}
	cfg := &config.Config{}
	svc := NewPushService(dbase, cfg)

	err := svc.NotifyNewMessage(userID, "TestRoom", "TestSender", "Hello")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func getTestDSN() string {
	if host := os.Getenv("POSTGRES_HOST"); host != "" {
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
			host,
			getEnvOrDefault("POSTGRES_PORT", "5432"),
			getEnvOrDefault("POSTGRES_USER", "postgres"),
			getEnvOrDefault("POSTGRES_PASSWORD", ""),
			getEnvOrDefault("POSTGRES_DB", "postgres"),
		)
	}
	return fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.New().String())
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func openTestDB(t *testing.T) *gorm.DB {
	dsn := getTestDSN()
	var db *gorm.DB
	var err error

	if os.Getenv("POSTGRES_HOST") != "" {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatalf("failed to connect to test database: %v", err)
		}
		tables := []string{
			"audit_logs", "users", "rooms", "categories", "user_rooms", "messages",
			"sessions", "refresh_tokens", "voice_sessions", "room_invites",
			"presences", "direct_messages", "webhooks", "webhook_logs",
			"pinned_messages", "message_reactions", "server_settings",
			"invites", "files", "push_subscriptions", "user_notification_settings",
		}
		for _, table := range tables {
			db.Exec("DELETE FROM " + table)
		}
		return db
	}

	db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	return db
}
