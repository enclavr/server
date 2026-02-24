package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RoomHandler struct {
	db *database.Database
}

func NewRoomHandler(db *database.Database) *RoomHandler {
	return &RoomHandler{db: db}
}

type CreateRoomRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Password    string `json:"password"`
	IsPrivate   bool   `json:"is_private"`
	MaxUsers    int    `json:"max_users"`
}

type RoomResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsPrivate   bool      `json:"is_private"`
	MaxUsers    int       `json:"max_users"`
	CreatedBy   uuid.UUID `json:"created_by"`
	CreatedAt   string    `json:"created_at"`
	UserCount   int       `json:"user_count"`
}

func (h *RoomHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Room name is required", http.StatusBadRequest)
		return
	}

	if req.MaxUsers == 0 {
		req.MaxUsers = 50
	}

	room := models.Room{
		Name:        req.Name,
		Description: req.Description,
		IsPrivate:   req.IsPrivate,
		MaxUsers:    req.MaxUsers,
		CreatedBy:   userID,
	}

	if req.Password != "" {
		room.Password = req.Password
	}

	if err := h.db.Create(&room).Error; err != nil {
		if err == gorm.ErrDuplicatedKey {
			http.Error(w, "Room name already exists", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to create room", http.StatusInternalServerError)
		return
	}

	userRoom := models.UserRoom{
		UserID: userID,
		RoomID: room.ID,
		Role:   "owner",
	}
	h.db.Create(&userRoom)

	h.sendRoomResponse(w, &room, 0)
}

func (h *RoomHandler) GetRooms(w http.ResponseWriter, r *http.Request) {
	var rooms []models.Room
	if err := h.db.Find(&rooms).Error; err != nil {
		http.Error(w, "Failed to fetch rooms", http.StatusInternalServerError)
		return
	}

	responses := make([]RoomResponse, 0, len(rooms))
	for _, room := range rooms {
		var userCount int64
		h.db.Model(&models.UserRoom{}).Where("room_id = ?", room.ID).Count(&userCount)
		responses = append(responses, h.roomToResponse(&room, int(userCount)))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

func (h *RoomHandler) GetRoom(w http.ResponseWriter, r *http.Request) {
	roomIDStr := r.URL.Query().Get("id")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	var room models.Room
	if err := h.db.First(&room, roomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	var userCount int64
	h.db.Model(&models.UserRoom{}).Where("room_id = ?", room.ID).Count(&userCount)

	h.sendRoomResponse(w, &room, int(userCount))
}

func (h *RoomHandler) JoinRoom(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		RoomID   uuid.UUID `json:"room_id"`
		Password string    `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var room models.Room
	if err := h.db.First(&room, req.RoomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	if room.IsPrivate && room.Password != "" {
		if room.Password != req.Password {
			http.Error(w, "Invalid password", http.StatusForbidden)
			return
		}
	}

	var existingUser models.UserRoom
	result := h.db.Where("user_id = ? AND room_id = ?", userID, req.RoomID).First(&existingUser)
	if result.Error == nil {
		http.Error(w, "Already in room", http.StatusConflict)
		return
	}

	var userCount int64
	h.db.Model(&models.UserRoom{}).Where("room_id = ?", req.RoomID).Count(&userCount)
	if int(userCount) >= room.MaxUsers {
		http.Error(w, "Room is full", http.StatusForbidden)
		return
	}

	userRoom := models.UserRoom{
		UserID: userID,
		RoomID: req.RoomID,
		Role:   "member",
	}
	h.db.Create(&userRoom)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "joined"})
}

func (h *RoomHandler) LeaveRoom(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		RoomID uuid.UUID `json:"room_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	result := h.db.Where("user_id = ? AND room_id = ?", userID, req.RoomID).Delete(&models.UserRoom{})
	if result.Error != nil {
		http.Error(w, "Failed to leave room", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Not in room", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "left"})
}

func (h *RoomHandler) roomToResponse(room *models.Room, userCount int) RoomResponse {
	return RoomResponse{
		ID:          room.ID,
		Name:        room.Name,
		Description: room.Description,
		IsPrivate:   room.IsPrivate,
		MaxUsers:    room.MaxUsers,
		CreatedBy:   room.CreatedBy,
		CreatedAt:   room.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UserCount:   userCount,
	}
}

func (h *RoomHandler) sendRoomResponse(w http.ResponseWriter, room *models.Room, userCount int) {
	response := h.roomToResponse(room, userCount)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
