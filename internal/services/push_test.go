package services

import (
	"testing"

	"github.com/enclavr/server/internal/config"
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
