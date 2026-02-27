package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to sqlite: %v", err)
	}
	return db
}

func TestUser_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	user := &User{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hash",
	}

	err := db.Create(user).Error
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if user.ID == uuid.Nil {
		t.Error("expected user ID to be set")
	}

	if user.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestUser_BeforeCreate_WithExistingID(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	existingID := uuid.New()
	user := &User{
		ID:           existingID,
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hash",
	}

	err := db.Create(user).Error
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if user.ID != existingID {
		t.Errorf("expected ID to be %s, got %s", existingID, user.ID)
	}
}

func TestRoom_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Room{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	room := &Room{
		Name:        "testroom",
		Description: "A test room",
		CreatedBy:   uuid.New(),
	}

	err := db.Create(room).Error
	if err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	if room.ID == uuid.Nil {
		t.Error("expected room ID to be set")
	}
}

func TestCategory_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Category{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	category := &Category{
		Name:      "testcategory",
		SortOrder: 1,
	}

	err := db.Create(category).Error
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	if category.ID == uuid.Nil {
		t.Error("expected category ID to be set")
	}
}

func TestUserRoom_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&UserRoom{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	userRoom := &UserRoom{
		UserID: uuid.New(),
		RoomID: uuid.New(),
		Role:   "member",
	}

	err := db.Create(userRoom).Error
	if err != nil {
		t.Fatalf("failed to create user room: %v", err)
	}

	if userRoom.ID == uuid.Nil {
		t.Error("expected user room ID to be set")
	}

	if userRoom.JoinedAt.IsZero() {
		t.Error("expected JoinedAt to be set")
	}
}

func TestUserRoom_BeforeCreate_WithExistingJoinedAt(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&UserRoom{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	joinedAt := time.Now().Add(-1 * time.Hour)
	userRoom := &UserRoom{
		UserID:   uuid.New(),
		RoomID:   uuid.New(),
		Role:     "member",
		JoinedAt: joinedAt,
	}

	err := db.Create(userRoom).Error
	if err != nil {
		t.Fatalf("failed to create user room: %v", err)
	}

	if !userRoom.JoinedAt.Equal(joinedAt) {
		t.Errorf("expected JoinedAt to be %v, got %v", joinedAt, userRoom.JoinedAt)
	}
}

func TestSession_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Session{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	session := &Session{
		UserID:    uuid.New(),
		Token:     "sometoken",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err := db.Create(session).Error
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if session.ID == uuid.Nil {
		t.Error("expected session ID to be set")
	}
}

func TestRefreshToken_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&RefreshToken{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	token := &RefreshToken{
		UserID:    uuid.New(),
		Token:     "refreshtoken",
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}

	err := db.Create(token).Error
	if err != nil {
		t.Fatalf("failed to create refresh token: %v", err)
	}

	if token.ID == uuid.Nil {
		t.Error("expected refresh token ID to be set")
	}
}

func TestMessage_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Room{}, &User{}, &Message{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	roomID := uuid.New()
	userID := uuid.New()
	db.Create(&Room{ID: roomID, Name: "test", CreatedBy: userID})
	db.Create(&User{ID: userID, Username: "user", Email: "a@b.com", PasswordHash: "h"})

	msg := &Message{
		RoomID:  roomID,
		UserID:  userID,
		Content: "Hello world",
	}

	err := db.Create(msg).Error
	if err != nil {
		t.Fatalf("failed to create message: %v", err)
	}

	if msg.ID == uuid.Nil {
		t.Error("expected message ID to be set")
	}
}

func TestMessage_BeforeCreate_WithExistingID(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Room{}, &User{}, &Message{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	roomID := uuid.New()
	userID := uuid.New()
	existingID := uuid.New()
	db.Create(&Room{ID: roomID, Name: "test", CreatedBy: userID})
	db.Create(&User{ID: userID, Username: "user", Email: "a@b.com", PasswordHash: "h"})

	msg := &Message{
		ID:        existingID,
		RoomID:    roomID,
		UserID:    userID,
		Content:   "Hello world",
		CreatedAt: time.Now(),
	}

	err := db.Create(msg).Error
	if err != nil {
		t.Fatalf("failed to create message: %v", err)
	}

	if msg.ID != existingID {
		t.Errorf("expected ID to be %s, got %s", existingID, msg.ID)
	}
}

func TestDirectMessage_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&DirectMessage{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	dm := &DirectMessage{
		SenderID:   uuid.New(),
		ReceiverID: uuid.New(),
		Content:    "Hello",
	}

	err := db.Create(dm).Error
	if err != nil {
		t.Fatalf("failed to create dm: %v", err)
	}

	if dm.ID == uuid.Nil {
		t.Error("expected dm ID to be set")
	}
}

func TestPinnedMessage_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&PinnedMessage{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	pinned := &PinnedMessage{
		MessageID: uuid.New(),
		RoomID:    uuid.New(),
		PinnedBy:  uuid.New(),
	}

	err := db.Create(pinned).Error
	if err != nil {
		t.Fatalf("failed to create pinned message: %v", err)
	}

	if pinned.ID == uuid.Nil {
		t.Error("expected pinned message ID to be set")
	}
}

func TestInvite_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Invite{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	invite := &Invite{
		RoomID:    uuid.New(),
		CreatedBy: uuid.New(),
	}

	err := db.Create(invite).Error
	if err != nil {
		t.Fatalf("failed to create invite: %v", err)
	}

	if invite.ID == uuid.Nil {
		t.Error("expected invite ID to be set")
	}

	if invite.Code == "" {
		t.Error("expected invite code to be generated")
	}
}

func TestInvite_BeforeCreate_WithExistingCode(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Invite{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	existingCode := "existingcode"
	invite := &Invite{
		RoomID:    uuid.New(),
		CreatedBy: uuid.New(),
		Code:      existingCode,
	}

	err := db.Create(invite).Error
	if err != nil {
		t.Fatalf("failed to create invite: %v", err)
	}

	if invite.Code != existingCode {
		t.Errorf("expected Code to be %s, got %s", existingCode, invite.Code)
	}
}

func TestMessageReaction_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&MessageReaction{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	reaction := &MessageReaction{
		MessageID: uuid.New(),
		UserID:    uuid.New(),
		Emoji:     "👍",
	}

	err := db.Create(reaction).Error
	if err != nil {
		t.Fatalf("failed to create reaction: %v", err)
	}

	if reaction.ID == uuid.Nil {
		t.Error("expected reaction ID to be set")
	}
}

func TestServerSettings_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&ServerSettings{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	settings := &ServerSettings{
		ServerName: "Test Server",
	}

	err := db.Create(settings).Error
	if err != nil {
		t.Fatalf("failed to create settings: %v", err)
	}

	if settings.ID == uuid.Nil {
		t.Error("expected settings ID to be set")
	}
}

func TestWebhook_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Webhook{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	webhook := &Webhook{
		RoomID:   uuid.New(),
		URL:      "https://example.com/webhook",
		Events:   "message.created",
		IsActive: true,
	}

	err := db.Create(webhook).Error
	if err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	if webhook.ID == uuid.Nil {
		t.Error("expected webhook ID to be set")
	}

	if webhook.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestWebhookLog_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&WebhookLog{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	log := &WebhookLog{
		WebhookID: uuid.New(),
		Event:     "message.created",
		Payload:   `{"test": true}`,
		Success:   true,
	}

	err := db.Create(log).Error
	if err != nil {
		t.Fatalf("failed to create webhook log: %v", err)
	}

	if log.ID == uuid.Nil {
		t.Error("expected webhook log ID to be set")
	}
}

func TestThread_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Thread{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	thread := &Thread{
		RoomID:    uuid.New(),
		ParentID:  uuid.New(),
		CreatedBy: uuid.New(),
	}

	err := db.Create(thread).Error
	if err != nil {
		t.Fatalf("failed to create thread: %v", err)
	}

	if thread.ID == uuid.Nil {
		t.Error("expected thread ID to be set")
	}
}

func TestThreadMessage_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&ThreadMessage{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	msg := &ThreadMessage{
		ThreadID: uuid.New(),
		UserID:   uuid.New(),
		Content:  "Thread reply",
	}

	err := db.Create(msg).Error
	if err != nil {
		t.Fatalf("failed to create thread message: %v", err)
	}

	if msg.ID == uuid.Nil {
		t.Error("expected thread message ID to be set")
	}
}

func TestVoiceSession_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&VoiceSession{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	voice := &VoiceSession{
		RoomID: uuid.New(),
		UserID: uuid.New(),
	}

	err := db.Create(voice).Error
	if err != nil {
		t.Fatalf("failed to create voice session: %v", err)
	}

	if voice.ID == uuid.Nil {
		t.Error("expected voice session ID to be set")
	}

	if voice.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
}

func TestRoomInvite_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&RoomInvite{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	invite := &RoomInvite{
		RoomID:    uuid.New(),
		CreatedBy: uuid.New(),
	}

	err := db.Create(invite).Error
	if err != nil {
		t.Fatalf("failed to create room invite: %v", err)
	}

	if invite.ID == uuid.Nil {
		t.Error("expected room invite ID to be set")
	}
}

func TestPresence_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Presence{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	presence := &Presence{
		UserID: uuid.New(),
		Status: PresenceOnline,
	}

	err := db.Create(presence).Error
	if err != nil {
		t.Fatalf("failed to create presence: %v", err)
	}

	if presence.ID == uuid.Nil {
		t.Error("expected presence ID to be set")
	}

	if presence.LastSeen.IsZero() {
		t.Error("expected LastSeen to be set")
	}
}

func TestFile_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&File{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	file := &File{
		UserID:      uuid.New(),
		RoomID:      uuid.New(),
		FileName:    "test.txt",
		FileSize:    1024,
		ContentType: "text/plain",
		StorageKey:  "uploads/test.txt",
	}

	err := db.Create(file).Error
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if file.ID == uuid.Nil {
		t.Error("expected file ID to be set")
	}

	if file.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}
