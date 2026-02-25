package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type ThreadHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewThreadHandler(db *database.Database, hub *websocket.Hub) *ThreadHandler {
	return &ThreadHandler{db: db, hub: hub}
}

type CreateThreadRequest struct {
	ParentID uuid.UUID `json:"parent_id"`
	RoomID   uuid.UUID `json:"room_id"`
	Content  string    `json:"content"`
}

type ThreadMessageRequest struct {
	Content string `json:"content"`
}

type ThreadResponse struct {
	ID        uuid.UUID               `json:"id"`
	RoomID    uuid.UUID               `json:"room_id"`
	ParentID  uuid.UUID               `json:"parent_id"`
	CreatedBy uuid.UUID               `json:"created_by"`
	Messages  []ThreadMessageResponse `json:"messages"`
	CreatedAt time.Time               `json:"created_at"`
}

type ThreadMessageResponse struct {
	ID        uuid.UUID `json:"id"`
	ThreadID  uuid.UUID `json:"thread_id"`
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	IsEdited  bool      `json:"is_edited"`
	IsDeleted bool      `json:"is_deleted"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (h *ThreadHandler) CreateThread(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateThreadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ParentID == uuid.Nil {
		http.Error(w, "parent_id is required", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Thread initial message content cannot be empty", http.StatusBadRequest)
		return
	}

	var parentMsg models.Message
	if err := h.db.First(&parentMsg, "id = ?", req.ParentID).Error; err != nil {
		http.Error(w, "Parent message not found", http.StatusNotFound)
		return
	}

	if parentMsg.RoomID != req.RoomID {
		http.Error(w, "Room mismatch", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, req.RoomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	thread := &models.Thread{
		RoomID:    req.RoomID,
		ParentID:  req.ParentID,
		CreatedBy: userID,
	}

	if err := h.db.Create(thread).Error; err != nil {
		log.Printf("Error creating thread: %v", err)
		http.Error(w, "Failed to create thread", http.StatusInternalServerError)
		return
	}

	threadMsg := &models.ThreadMessage{
		ThreadID: thread.ID,
		UserID:   userID,
		Content:  req.Content,
	}

	if err := h.db.Create(threadMsg).Error; err != nil {
		log.Printf("Error creating thread message: %v", err)
		http.Error(w, "Failed to create thread message", http.StatusInternalServerError)
		return
	}

	var user models.User
	h.db.First(&user, "id = ?", userID)

	response := ThreadResponse{
		ID:        thread.ID,
		RoomID:    thread.RoomID,
		ParentID:  thread.ParentID,
		CreatedBy: thread.CreatedBy,
		CreatedAt: thread.CreatedAt,
		Messages: []ThreadMessageResponse{
			{
				ID:        threadMsg.ID,
				ThreadID:  threadMsg.ThreadID,
				UserID:    threadMsg.UserID,
				Username:  user.Username,
				Content:   threadMsg.Content,
				IsEdited:  threadMsg.IsEdited,
				IsDeleted: threadMsg.IsDeleted,
				CreatedAt: threadMsg.CreatedAt,
				UpdatedAt: threadMsg.UpdatedAt,
			},
		},
	}

	wsMsg := &websocket.Message{
		Type:      "thread-created",
		RoomID:    req.RoomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(response)
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(req.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ThreadHandler) GetThread(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	threadIDStr := r.URL.Query().Get("thread_id")
	if threadIDStr == "" {
		http.Error(w, "thread_id is required", http.StatusBadRequest)
		return
	}

	threadID, err := uuid.Parse(threadIDStr)
	if err != nil {
		http.Error(w, "Invalid thread_id", http.StatusBadRequest)
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, "id = ?", threadID).Error; err != nil {
		http.Error(w, "Thread not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, thread.RoomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var threadMessages []models.ThreadMessage
	if err := h.db.Where("thread_id = ?", threadID).Order("created_at ASC").Find(&threadMessages).Error; err != nil {
		http.Error(w, "Failed to fetch thread messages", http.StatusInternalServerError)
		return
	}

	var messages []ThreadMessageResponse
	for _, msg := range threadMessages {
		var user models.User
		h.db.First(&user, "id = ?", msg.UserID)

		content := msg.Content
		if msg.IsDeleted {
			content = ""
		}

		messages = append(messages, ThreadMessageResponse{
			ID:        msg.ID,
			ThreadID:  msg.ThreadID,
			UserID:    msg.UserID,
			Username:  user.Username,
			Content:   content,
			IsEdited:  msg.IsEdited,
			IsDeleted: msg.IsDeleted,
			CreatedAt: msg.CreatedAt,
			UpdatedAt: msg.UpdatedAt,
		})
	}

	response := ThreadResponse{
		ID:        thread.ID,
		RoomID:    thread.RoomID,
		ParentID:  thread.ParentID,
		CreatedBy: thread.CreatedBy,
		Messages:  messages,
		CreatedAt: thread.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ThreadHandler) GetThreadsForMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	messageIDStr := r.URL.Query().Get("message_id")
	if messageIDStr == "" {
		http.Error(w, "message_id is required", http.StatusBadRequest)
		return
	}

	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		http.Error(w, "Invalid message_id", http.StatusBadRequest)
		return
	}

	var parentMsg models.Message
	if err := h.db.First(&parentMsg, "id = ?", messageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, parentMsg.RoomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var threads []models.Thread
	if err := h.db.Where("parent_id = ?", messageID).Order("created_at DESC").Find(&threads).Error; err != nil {
		http.Error(w, "Failed to fetch threads", http.StatusInternalServerError)
		return
	}

	type ThreadListItem struct {
		ID           uuid.UUID `json:"id"`
		RoomID       uuid.UUID `json:"room_id"`
		ParentID     uuid.UUID `json:"parent_id"`
		CreatedBy    uuid.UUID `json:"created_by"`
		Username     string    `json:"username"`
		MessageCount int       `json:"message_count"`
		LastMessage  string    `json:"last_message"`
		CreatedAt    time.Time `json:"created_at"`
	}

	var result []ThreadListItem
	for _, thread := range threads {
		var user models.User
		h.db.First(&user, "id = ?", thread.CreatedBy)

		var count int64
		h.db.Model(&models.ThreadMessage{}).Where("thread_id = ?", thread.ID).Count(&count)

		var lastMsg models.ThreadMessage
		h.db.Where("thread_id = ?", thread.ID).Order("created_at DESC").First(&lastMsg)

		lastContent := ""
		if !lastMsg.IsDeleted {
			lastContent = lastMsg.Content
		}

		result = append(result, ThreadListItem{
			ID:           thread.ID,
			RoomID:       thread.RoomID,
			ParentID:     thread.ParentID,
			CreatedBy:    thread.CreatedBy,
			Username:     user.Username,
			MessageCount: int(count),
			LastMessage:  lastContent,
			CreatedAt:    thread.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ThreadHandler) AddThreadMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	threadIDStr := r.URL.Query().Get("thread_id")
	if threadIDStr == "" {
		http.Error(w, "thread_id is required", http.StatusBadRequest)
		return
	}

	threadID, err := uuid.Parse(threadIDStr)
	if err != nil {
		http.Error(w, "Invalid thread_id", http.StatusBadRequest)
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, "id = ?", threadID).Error; err != nil {
		http.Error(w, "Thread not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, thread.RoomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var req ThreadMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Message content cannot be empty", http.StatusBadRequest)
		return
	}

	threadMsg := &models.ThreadMessage{
		ThreadID: threadID,
		UserID:   userID,
		Content:  req.Content,
	}

	if err := h.db.Create(threadMsg).Error; err != nil {
		log.Printf("Error creating thread message: %v", err)
		http.Error(w, "Failed to create message", http.StatusInternalServerError)
		return
	}

	var user models.User
	h.db.First(&user, "id = ?", userID)

	response := ThreadMessageResponse{
		ID:        threadMsg.ID,
		ThreadID:  threadMsg.ThreadID,
		UserID:    threadMsg.UserID,
		Username:  user.Username,
		Content:   threadMsg.Content,
		IsEdited:  threadMsg.IsEdited,
		IsDeleted: threadMsg.IsDeleted,
		CreatedAt: threadMsg.CreatedAt,
		UpdatedAt: threadMsg.UpdatedAt,
	}

	wsMsg := &websocket.Message{
		Type:      "thread-message",
		RoomID:    thread.RoomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(response)
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(thread.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ThreadHandler) UpdateThreadMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	messageIDStr := r.URL.Query().Get("message_id")
	if messageIDStr == "" {
		http.Error(w, "message_id is required", http.StatusBadRequest)
		return
	}

	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		http.Error(w, "Invalid message_id", http.StatusBadRequest)
		return
	}

	var req ThreadMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Message content cannot be empty", http.StatusBadRequest)
		return
	}

	var threadMsg models.ThreadMessage
	if err := h.db.First(&threadMsg, "id = ?", messageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if threadMsg.UserID != userID {
		http.Error(w, "You can only edit your own messages", http.StatusForbidden)
		return
	}

	threadMsg.Content = req.Content
	threadMsg.IsEdited = true
	threadMsg.UpdatedAt = time.Now()

	if err := h.db.Save(&threadMsg).Error; err != nil {
		http.Error(w, "Failed to update message", http.StatusInternalServerError)
		return
	}

	var user models.User
	h.db.First(&user, "id = ?", threadMsg.UserID)

	response := ThreadMessageResponse{
		ID:        threadMsg.ID,
		ThreadID:  threadMsg.ThreadID,
		UserID:    threadMsg.UserID,
		Username:  user.Username,
		Content:   threadMsg.Content,
		IsEdited:  threadMsg.IsEdited,
		IsDeleted: threadMsg.IsDeleted,
		CreatedAt: threadMsg.CreatedAt,
		UpdatedAt: threadMsg.UpdatedAt,
	}

	wsMsg := &websocket.Message{
		Type:      "thread-message-updated",
		RoomID:    uuid.Nil,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(response)
	wsMsg.Payload = wsPayload

	var thread models.Thread
	h.db.First(&thread, "id = ?", threadMsg.ThreadID)
	h.hub.BroadcastToRoom(thread.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ThreadHandler) DeleteThreadMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	messageIDStr := r.URL.Query().Get("message_id")
	if messageIDStr == "" {
		http.Error(w, "message_id is required", http.StatusBadRequest)
		return
	}

	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		http.Error(w, "Invalid message_id", http.StatusBadRequest)
		return
	}

	var threadMsg models.ThreadMessage
	if err := h.db.First(&threadMsg, "id = ?", messageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if threadMsg.UserID != userID {
		http.Error(w, "You can only delete your own messages", http.StatusForbidden)
		return
	}

	threadMsg.IsDeleted = true
	threadMsg.Content = ""
	threadMsg.UpdatedAt = time.Now()

	if err := h.db.Save(&threadMsg).Error; err != nil {
		http.Error(w, "Failed to delete message", http.StatusInternalServerError)
		return
	}

	wsMsg := &websocket.Message{
		Type:      "thread-message-deleted",
		RoomID:    uuid.Nil,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(map[string]string{"id": messageID.String()})
	wsMsg.Payload = wsPayload

	var thread models.Thread
	h.db.First(&thread, "id = ?", threadMsg.ThreadID)
	h.hub.BroadcastToRoom(thread.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
