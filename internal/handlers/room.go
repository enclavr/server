package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type RoomHandler struct {
	db *database.Database
}

func NewRoomHandler(db *database.Database) *RoomHandler {
	return &RoomHandler{db: db}
}

type CreateRoomRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Password    string     `json:"password"`
	IsPrivate   bool       `json:"is_private"`
	MaxUsers    int        `json:"max_users"`
	CategoryID  *uuid.UUID `json:"category_id"`
}

type RoomResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	IsPrivate   bool       `json:"is_private"`
	MaxUsers    int        `json:"max_users"`
	CreatedBy   uuid.UUID  `json:"created_by"`
	CategoryID  *uuid.UUID `json:"category_id,omitempty"`
	CreatedAt   string     `json:"created_at"`
	UserCount   int        `json:"user_count"`
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
	if req.MaxUsers < 0 {
		http.Error(w, "Max users must be a positive number", http.StatusBadRequest)
		return
	}

	room := models.Room{
		Name:        req.Name,
		Description: req.Description,
		IsPrivate:   req.IsPrivate,
		MaxUsers:    req.MaxUsers,
		CreatedBy:   userID,
		CategoryID:  req.CategoryID,
	}

	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to hash password", http.StatusInternalServerError)
			return
		}
		room.Password = string(hashedPassword)
	}

	if err := h.db.Create(&room).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
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
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var rooms []models.Room
	if err := h.db.Where("is_private = ? OR id IN (SELECT room_id FROM user_rooms WHERE user_id = ?)", false, userID).Find(&rooms).Error; err != nil {
		http.Error(w, "Failed to fetch rooms", http.StatusInternalServerError)
		return
	}

	roomIDs := make([]uuid.UUID, len(rooms))
	for i, room := range rooms {
		roomIDs[i] = room.ID
	}

	type countResult struct {
		RoomID uuid.UUID
		Count  int64
	}
	var counts []countResult
	h.db.Model(&models.UserRoom{}).Select("room_id, count(*) as count").Where("room_id IN ?", roomIDs).Group("room_id").Scan(&counts)

	countMap := make(map[uuid.UUID]int, len(counts))
	for _, c := range counts {
		countMap[c.RoomID] = int(c.Count)
	}

	responses := make([]RoomResponse, 0, len(rooms))
	for _, room := range rooms {
		responses = append(responses, h.roomToResponse(&room, countMap[room.ID]))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoomHandler) GetRoom(w http.ResponseWriter, r *http.Request) {
	roomIDStr := r.URL.Query().Get("id")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil || roomID == uuid.Nil {
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
		if err := bcrypt.CompareHashAndPassword([]byte(room.Password), []byte(req.Password)); err != nil {
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

	tx := h.db.Begin()

	var userCount int64
	tx.Model(&models.UserRoom{}).Where("room_id = ?", req.RoomID).Count(&userCount)
	if int(userCount) >= room.MaxUsers {
		tx.Rollback()
		http.Error(w, "Room is full", http.StatusForbidden)
		return
	}

	userRoom := models.UserRoom{
		UserID: userID,
		RoomID: req.RoomID,
		Role:   "member",
	}
	if err := tx.Create(&userRoom).Error; err != nil {
		tx.Rollback()
		http.Error(w, "Failed to join room", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit().Error; err != nil {
		http.Error(w, "Failed to join room", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "joined"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
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
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "left"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoomHandler) roomToResponse(room *models.Room, userCount int) RoomResponse {
	return RoomResponse{
		ID:          room.ID,
		Name:        room.Name,
		Description: room.Description,
		IsPrivate:   room.IsPrivate,
		MaxUsers:    room.MaxUsers,
		CreatedBy:   room.CreatedBy,
		CategoryID:  room.CategoryID,
		CreatedAt:   room.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UserCount:   userCount,
	}
}

func (h *RoomHandler) sendRoomResponse(w http.ResponseWriter, room *models.Room, userCount int) {
	response := h.roomToResponse(room, userCount)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

type SearchRoomsRequest struct {
	Query      string     `json:"query"`
	CategoryID *uuid.UUID `json:"category_id"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
}

type SearchRoomsResponse struct {
	Rooms  []RoomResponse `json:"rooms"`
	Total  int64          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

func (h *RoomHandler) SearchRooms(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req SearchRoomsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 20
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	query := h.db.Where("is_private = ?", false)

	if req.Query != "" {
		searchPattern := "%" + req.Query + "%"
		query = query.Where("name ILIKE ? OR description ILIKE ?", searchPattern, searchPattern)
	}

	if req.CategoryID != nil {
		query = query.Where("category_id = ?", req.CategoryID)
	}

	var total int64
	if err := query.Model(&models.Room{}).Count(&total).Error; err != nil {
		http.Error(w, "Failed to count rooms", http.StatusInternalServerError)
		return
	}

	var rooms []models.Room
	if err := query.
		Order("created_at DESC").
		Offset(req.Offset).
		Limit(req.Limit).
		Find(&rooms).Error; err != nil {
		http.Error(w, "Failed to search rooms", http.StatusInternalServerError)
		return
	}

	roomResponses := make([]RoomResponse, 0, len(rooms))
	for _, room := range rooms {
		var userCount int64
		h.db.Model(&models.UserRoom{}).Where("room_id = ?", room.ID).Count(&userCount)

		isMember := false
		result := h.db.Model(&models.UserRoom{}).Where("user_id = ? AND room_id = ?", userID, room.ID).First(&models.UserRoom{})
		if result.Error == nil {
			isMember = true
		}

		response := h.roomToResponse(&room, int(userCount))
		if isMember {
			roomResponses = append([]RoomResponse{response}, roomResponses...)
		} else {
			roomResponses = append(roomResponses, response)
		}
	}

	response := SearchRoomsResponse{
		Rooms:  roomResponses,
		Total:  total,
		Limit:  req.Limit,
		Offset: req.Offset,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

type UserRoomResponse struct {
	RoomID      uuid.UUID `json:"room_id"`
	RoomName    string    `json:"room_name"`
	Description string    `json:"description"`
	Role        string    `json:"role"`
	IsPrivate   bool      `json:"is_private"`
	JoinedAt    string    `json:"joined_at"`
	MemberCount int       `json:"member_count"`
}

func (h *RoomHandler) GetUserRooms(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var userRooms []models.UserRoom
	if err := h.db.Where("user_id = ?", userID).Find(&userRooms).Error; err != nil {
		http.Error(w, "Failed to fetch user rooms", http.StatusInternalServerError)
		return
	}

	roomIDs := make([]uuid.UUID, len(userRooms))
	for i, ur := range userRooms {
		roomIDs[i] = ur.RoomID
	}

	var rooms []models.Room
	if err := h.db.Where("id IN ?", roomIDs).Find(&rooms).Error; err != nil {
		http.Error(w, "Failed to fetch rooms", http.StatusInternalServerError)
		return
	}
	roomMap := make(map[uuid.UUID]models.Room, len(rooms))
	for _, room := range rooms {
		roomMap[room.ID] = room
	}

	type countResult struct {
		RoomID uuid.UUID
		Count  int64
	}
	var counts []countResult
	h.db.Model(&models.UserRoom{}).Select("room_id, count(*) as count").Where("room_id IN ?", roomIDs).Group("room_id").Scan(&counts)
	countMap := make(map[uuid.UUID]int, len(counts))
	for _, c := range counts {
		countMap[c.RoomID] = int(c.Count)
	}

	roomResponses := make([]UserRoomResponse, 0, len(userRooms))
	for _, ur := range userRooms {
		room, ok := roomMap[ur.RoomID]
		if !ok {
			continue
		}

		roomResp := UserRoomResponse{
			RoomID:      ur.RoomID,
			RoomName:    room.Name,
			Description: room.Description,
			Role:        ur.Role,
			IsPrivate:   room.IsPrivate,
			JoinedAt:    ur.JoinedAt.Format("2006-01-02T15:04:05Z07:00"),
			MemberCount: countMap[ur.RoomID],
		}
		roomResponses = append(roomResponses, roomResp)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(roomResponses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
