package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func setupReminderHandlerDB(t *testing.T) *database.Database {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Message{},
		&models.MessageReminder{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return &database.Database{DB: db}
}

func addReminderIDToPath(r *http.Request, reminderID uuid.UUID) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.ReminderIDKey, reminderID.String())
	return r.WithContext(ctx)
}

func TestReminderHandler_CreateAndGet(t *testing.T) {
	db := setupReminderHandlerDB(t)

	userID := uuid.New()
	roomID := uuid.New()
	messageID := uuid.New()

	user := models.User{
		ID:       userID,
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	room := models.Room{
		ID:        roomID,
		Name:      "test-room-" + uuid.New().String()[:8],
		CreatedBy: userID,
	}
	db.Create(&room)

	userRoom := models.UserRoom{
		UserID: userID,
		RoomID: roomID,
		Role:   "member",
	}
	db.Create(&userRoom)

	now := time.Now()
	message := models.Message{
		ID:        messageID,
		RoomID:    roomID,
		UserID:    userID,
		Content:   "Test message",
		CreatedAt: now,
		UpdatedAt: now,
	}
	db.Create(&message)

	handler := NewReminderHandler(db)

	t.Run("create reminder successfully", func(t *testing.T) {
		req := CreateReminderRequest{
			MessageID: messageID,
			RemindAt:  time.Now().Add(1 * time.Hour),
			Note:      "Test reminder",
		}
		body, _ := json.Marshal(req)

		httpReq := httptest.NewRequest(http.MethodPost, "/api/reminder", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(httpReq.Context(), middleware.UserIDKey, userID)
		httpReq = httpReq.WithContext(ctx)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httpReq

		handler.CreateReminder(c.Writer, c.Request)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status %d, got %d. Body: %s", http.StatusCreated, w.Code, w.Body.String())
		}

		var resp ReminderResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if resp.MessageID != messageID {
			t.Errorf("Expected message ID %s, got %s", messageID, resp.MessageID)
		}
		if resp.Note != "Test reminder" {
			t.Errorf("Expected note 'Test reminder', got '%s'", resp.Note)
		}
		if resp.IsTriggered {
			t.Error("Expected is_triggered to be false")
		}
	})

	t.Run("get all reminders", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/api/reminders", nil)
		ctx := context.WithValue(httpReq.Context(), middleware.UserIDKey, userID)
		httpReq = httpReq.WithContext(ctx)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httpReq

		handler.GetReminders(c.Writer, c.Request)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var reminders []ReminderResponse
		if err := json.NewDecoder(w.Body).Decode(&reminders); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if len(reminders) == 0 {
			t.Error("Expected at least one reminder")
		}
	})
}

func TestReminderHandler_Validation(t *testing.T) {
	db := setupReminderHandlerDB(t)

	handler := NewReminderHandler(db)
	userID := uuid.New()

	user := models.User{
		ID:       userID,
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	t.Run("missing message ID", func(t *testing.T) {
		req := CreateReminderRequest{
			MessageID: uuid.Nil,
			RemindAt:  time.Now().Add(1 * time.Hour),
		}
		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/reminder", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(httpReq.Context(), middleware.UserIDKey, userID)
		httpReq = httpReq.WithContext(ctx)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httpReq

		handler.CreateReminder(c.Writer, c.Request)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("past remind time", func(t *testing.T) {
		req := CreateReminderRequest{
			MessageID: uuid.New(),
			RemindAt:  time.Now().Add(-1 * time.Hour),
		}
		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/reminder", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(httpReq.Context(), middleware.UserIDKey, userID)
		httpReq = httpReq.WithContext(ctx)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httpReq

		handler.CreateReminder(c.Writer, c.Request)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("unauthorized without user ID", func(t *testing.T) {
		req := CreateReminderRequest{
			MessageID: uuid.New(),
			RemindAt:  time.Now().Add(1 * time.Hour),
		}
		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/api/reminder", bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httpReq

		handler.CreateReminder(c.Writer, c.Request)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}
	})
}

func TestReminderHandler_UpdateAndDelete(t *testing.T) {
	db := setupReminderHandlerDB(t)

	userID := uuid.New()
	roomID := uuid.New()
	messageID := uuid.New()

	user := models.User{
		ID:       userID,
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	room := models.Room{
		ID:        roomID,
		Name:      "test-room-update-" + uuid.New().String()[:8],
		CreatedBy: userID,
	}
	db.Create(&room)

	userRoom := models.UserRoom{
		UserID: userID,
		RoomID: roomID,
		Role:   "member",
	}
	db.Create(&userRoom)

	message := models.Message{
		ID:        messageID,
		RoomID:    roomID,
		UserID:    userID,
		Content:   "Test message",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.Create(&message)

	reminder := models.MessageReminder{
		ID:        uuid.New(),
		UserID:    userID,
		MessageID: messageID,
		RemindAt:  time.Now().Add(2 * time.Hour),
		Note:      "Original note",
	}
	db.Create(&reminder)

	handler := NewReminderHandler(db)
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)

	t.Run("update reminder", func(t *testing.T) {
		newNote := "Updated note"
		req := UpdateReminderRequest{
			Note: &newNote,
		}
		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest(http.MethodPut, "/api/reminder/"+reminder.ID.String(), bytes.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq = httpReq.WithContext(ctx)
		httpReq = addReminderIDToPath(httpReq, reminder.ID)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httpReq

		handler.UpdateReminder(c.Writer, c.Request)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp ReminderResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if resp.Note != "Updated note" {
			t.Errorf("Expected note 'Updated note', got '%s'", resp.Note)
		}
	})

	t.Run("delete reminder", func(t *testing.T) {
		newReminder := models.MessageReminder{
			ID:        uuid.New(),
			UserID:    userID,
			MessageID: messageID,
			RemindAt:  time.Now().Add(3 * time.Hour),
			Note:      "To be deleted",
		}
		db.Create(&newReminder)

		httpReq := httptest.NewRequest(http.MethodDelete, "/api/reminder/"+newReminder.ID.String(), nil)
		httpReq = httpReq.WithContext(ctx)
		httpReq = addReminderIDToPath(httpReq, newReminder.ID)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httpReq

		handler.DeleteReminder(w, c.Request)

		if w.Code != http.StatusNoContent {
			t.Errorf("Expected status %d, got %d", http.StatusNoContent, w.Code)
		}

		var existing models.MessageReminder
		err := db.Unscoped().First(&existing, "id = ?", newReminder.ID).Error
		if err == sql.ErrNoRows {
			t.Error("Expected reminder to be soft deleted (found with Unscoped)")
		}
	})
}

func TestReminderHandler_GetPendingReminders(t *testing.T) {
	db := setupReminderHandlerDB(t)

	userID := uuid.New()
	roomID := uuid.New()

	user := models.User{
		ID:       userID,
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	room := models.Room{
		ID:        roomID,
		Name:      "test-room-pending-" + uuid.New().String()[:8],
		CreatedBy: userID,
	}
	db.Create(&room)

	userRoom := models.UserRoom{
		UserID: userID,
		RoomID: roomID,
		Role:   "member",
	}
	db.Create(&userRoom)

	message := models.Message{
		ID:        uuid.New(),
		RoomID:    roomID,
		UserID:    userID,
		Content:   "Test message",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.Create(&message)

	pastTime := time.Now().Add(-1 * time.Hour)
	reminder := models.MessageReminder{
		ID:        uuid.New(),
		UserID:    userID,
		MessageID: message.ID,
		RemindAt:  pastTime,
		Note:      "Past reminder",
	}
	db.Create(&reminder)

	handler := NewReminderHandler(db)
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)
	req := httptest.NewRequest(http.MethodGet, "/api/reminders/pending", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetPendingReminders(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var reminders []ReminderResponse
	if err := json.NewDecoder(w.Body).Decode(&reminders); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(reminders) == 0 {
		t.Error("Expected at least one pending reminder")
	}
}

func TestReminderHandler_GetReminder(t *testing.T) {
	db := setupReminderHandlerDB(t)

	userID := uuid.New()

	user := models.User{
		ID:       userID,
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	reminder := models.MessageReminder{
		ID:        uuid.New(),
		UserID:    userID,
		MessageID: uuid.New(),
		RemindAt:  time.Now().Add(1 * time.Hour),
		Note:      "Test reminder",
	}
	db.Create(&reminder)

	handler := NewReminderHandler(db)
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)

	t.Run("get reminder by ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/reminder/"+reminder.ID.String(), nil)
		req = req.WithContext(ctx)
		req = addReminderIDToPath(req, reminder.ID)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.GetReminder(c.Writer, c.Request)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var resp ReminderResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if resp.ID != reminder.ID {
			t.Errorf("Expected ID %s, got %s", reminder.ID, resp.ID)
		}
	})

	t.Run("get non-existent reminder", func(t *testing.T) {
		nonExistentID := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/api/reminder/"+nonExistentID.String(), nil)
		req = req.WithContext(ctx)
		req = addReminderIDToPath(req, nonExistentID)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.GetReminder(c.Writer, c.Request)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})

	t.Run("get reminder with invalid ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/reminder/invalid-uuid", nil)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.GetReminder(c.Writer, c.Request)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})
}

func TestReminderHandler_GetReminders_Pending(t *testing.T) {
	db := setupReminderHandlerDB(t)

	userID := uuid.New()

	user := models.User{
		ID:       userID,
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	futureTime := time.Now().Add(1 * time.Hour)
	reminder := models.MessageReminder{
		ID:        uuid.New(),
		UserID:    userID,
		MessageID: uuid.New(),
		RemindAt:  futureTime,
		Note:      "Future reminder",
	}
	db.Create(&reminder)

	handler := NewReminderHandler(db)
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, userID)

	t.Run("get pending reminders only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/reminders?pending=true", nil)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler.GetReminders(c.Writer, c.Request)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var reminders []ReminderResponse
		if err := json.NewDecoder(w.Body).Decode(&reminders); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(reminders) != 1 {
			t.Errorf("Expected 1 pending reminder, got %d", len(reminders))
		}
	})
}
