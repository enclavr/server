package models

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	return openTestDB(t)
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
			"users", "rooms", "categories", "user_rooms", "messages",
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

func TestPoll_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Poll{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	poll := &Poll{
		Question:  "Test poll?",
		RoomID:    uuid.New(),
		CreatedBy: uuid.New(),
		ExpiresAt: &expiresAt,
	}

	err := db.Create(poll).Error
	if err != nil {
		t.Fatalf("failed to create poll: %v", err)
	}

	if poll.ID == uuid.Nil {
		t.Error("expected poll ID to be set")
	}

	if poll.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestPollOption_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Poll{}, &PollOption{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	pollID := uuid.New()
	db.Create(&Poll{ID: pollID, Question: "Test", RoomID: uuid.New(), CreatedBy: uuid.New()})

	opt := &PollOption{
		PollID: pollID,
		Text:   "Option 1",
	}

	err := db.Create(opt).Error
	if err != nil {
		t.Fatalf("failed to create poll option: %v", err)
	}

	if opt.ID == uuid.Nil {
		t.Error("expected poll option ID to be set")
	}

	if opt.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestPollVote_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Poll{}, &PollOption{}, &PollVote{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	pollID := uuid.New()
	optID := uuid.New()
	userID := uuid.New()
	db.Create(&Poll{ID: pollID, Question: "Test", RoomID: uuid.New(), CreatedBy: userID})
	db.Create(&PollOption{ID: optID, PollID: pollID, Text: "Option"})

	vote := &PollVote{
		PollID:   pollID,
		OptionID: optID,
		UserID:   userID,
	}

	err := db.Create(vote).Error
	if err != nil {
		t.Fatalf("failed to create poll vote: %v", err)
	}

	if vote.ID == uuid.Nil {
		t.Error("expected poll vote ID to be set")
	}

	if vote.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestServerEmoji_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&ServerEmoji{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	emoji := &ServerEmoji{
		Name:      "smile",
		ImageURL:  "https://example.com/emoji.png",
		CreatedBy: uuid.New(),
	}

	err := db.Create(emoji).Error
	if err != nil {
		t.Fatalf("failed to create emoji: %v", err)
	}

	if emoji.ID == uuid.Nil {
		t.Error("expected emoji ID to be set")
	}

	if emoji.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestServerSticker_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&ServerSticker{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	sticker := &ServerSticker{
		Name:      "funny",
		ImageURL:  "https://example.com/sticker.png",
		CreatedBy: uuid.New(),
	}

	err := db.Create(sticker).Error
	if err != nil {
		t.Fatalf("failed to create sticker: %v", err)
	}

	if sticker.ID == uuid.Nil {
		t.Error("expected sticker ID to be set")
	}

	if sticker.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestSoundboardSound_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&SoundboardSound{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	sound := &SoundboardSound{
		Name:      "ping",
		AudioURL:  "https://example.com/ping.mp3",
		CreatedBy: uuid.New(),
	}

	err := db.Create(sound).Error
	if err != nil {
		t.Fatalf("failed to create soundboard sound: %v", err)
	}

	if sound.ID == uuid.Nil {
		t.Error("expected sound ID to be set")
	}

	if sound.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestDailyAnalytics_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&DailyAnalytics{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	analytics := &DailyAnalytics{
		Date:          time.Now().Truncate(24 * time.Hour),
		TotalMessages: 100,
		TotalUsers:    10,
	}

	err := db.Create(analytics).Error
	if err != nil {
		t.Fatalf("failed to create daily analytics: %v", err)
	}

	if analytics.ID == uuid.Nil {
		t.Error("expected analytics ID to be set")
	}

	if analytics.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestHourlyActivity_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&HourlyActivity{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	activity := &HourlyActivity{
		Hour:         12,
		Date:         time.Now().Truncate(24 * time.Hour),
		MessageCount: 50,
		UserCount:    5,
	}

	err := db.Create(activity).Error
	if err != nil {
		t.Fatalf("failed to create hourly activity: %v", err)
	}

	if activity.ID == uuid.Nil {
		t.Error("expected activity ID to be set")
	}

	if activity.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestChannelActivity_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&ChannelActivity{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	activity := &ChannelActivity{
		RoomID:       uuid.New(),
		Date:         time.Now().Truncate(24 * time.Hour),
		MessageCount: 50,
		UserCount:    5,
	}

	err := db.Create(activity).Error
	if err != nil {
		t.Fatalf("failed to create channel activity: %v", err)
	}

	if activity.ID == uuid.Nil {
		t.Error("expected activity ID to be set")
	}

	if activity.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestAuditLog_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&AuditLog{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	log := &AuditLog{
		UserID:     uuid.New(),
		Action:     AuditActionUserBan,
		TargetType: "user",
		TargetID:   uuid.New(),
		Details:    `{"reason": "spam"}`,
	}

	err := db.Create(log).Error
	if err != nil {
		t.Fatalf("failed to create audit log: %v", err)
	}

	if log.ID == uuid.Nil {
		t.Error("expected audit log ID to be set")
	}

	if log.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestBan_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Ban{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ban := &Ban{
		UserID:   uuid.New(),
		RoomID:   uuid.New(),
		BannedBy: uuid.New(),
		Reason:   "violation",
	}

	err := db.Create(ban).Error
	if err != nil {
		t.Fatalf("failed to create ban: %v", err)
	}

	if ban.ID == uuid.Nil {
		t.Error("expected ban ID to be set")
	}

	if ban.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestReport_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&Report{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	report := &Report{
		ReporterID: uuid.New(),
		ReportedID: uuid.New(),
		RoomID:     uuid.New(),
		Reason:     "inappropriate content",
		Status:     "pending",
	}

	err := db.Create(report).Error
	if err != nil {
		t.Fatalf("failed to create report: %v", err)
	}

	if report.ID == uuid.Nil {
		t.Error("expected report ID to be set")
	}

	if report.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestPushSubscription_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&PushSubscription{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	sub := &PushSubscription{
		UserID:   uuid.New(),
		Endpoint: "https://example.com/push",
		P256DH:   "testp256dh",
		Auth:     "testauth",
	}

	err := db.Create(sub).Error
	if err != nil {
		t.Fatalf("failed to create push subscription: %v", err)
	}

	if sub.ID == uuid.Nil {
		t.Error("expected subscription ID to be set")
	}

	if sub.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestUserNotificationSettings_BeforeCreate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&UserNotificationSettings{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	settings := &UserNotificationSettings{
		UserID: uuid.New(),
	}

	err := db.Create(settings).Error
	if err != nil {
		t.Fatalf("failed to create notification settings: %v", err)
	}

	if settings.ID == uuid.Nil {
		t.Error("expected settings ID to be set")
	}

	if settings.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
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
