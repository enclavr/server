package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ConnectionHandler struct {
	db *database.Database
}

func NewConnectionHandler(db *database.Database) *ConnectionHandler {
	return &ConnectionHandler{db: db}
}

type ConnectionRequest struct {
	ConnectedUserID uuid.UUID `json:"connected_user_id"`
}

type ConnectionResponse struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	ConnectedUserID uuid.UUID `json:"connected_user_id"`
	Username        string    `json:"username"`
	DisplayName     string    `json:"display_name"`
	AvatarURL       string    `json:"avatar_url"`
	Status          string    `json:"status"`
	Direction       string    `json:"direction"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
}

type ConnectionListResponse struct {
	Connections []ConnectionResponse `json:"connections"`
	Total       int                  `json:"total"`
}

func (h *ConnectionHandler) SendRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ConnectedUserID == uuid.Nil {
		http.Error(w, "connected_user_id is required", http.StatusBadRequest)
		return
	}

	if req.ConnectedUserID == userID {
		http.Error(w, "Cannot send connection request to yourself", http.StatusBadRequest)
		return
	}

	var targetUser models.User
	if err := h.db.First(&targetUser, "id = ?", req.ConnectedUserID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	var existingConnection models.UserConnection
	err := h.db.Where(
		"(user_id = ? AND connected_user_id = ?) OR (user_id = ? AND connected_user_id = ?)",
		userID, req.ConnectedUserID, req.ConnectedUserID, userID,
	).First(&existingConnection).Error

	if err == nil {
		switch existingConnection.Status {
		case models.ConnectionStatusAccepted:
			http.Error(w, "Already connected", http.StatusConflict)
			return
		case models.ConnectionStatusPending:
			http.Error(w, "Connection request already pending", http.StatusConflict)
			return
		case models.ConnectionStatusBlocked:
			http.Error(w, "Cannot send connection request", http.StatusForbidden)
			return
		}
	}

	connection := &models.UserConnection{
		UserID:          userID,
		ConnectedUserID: req.ConnectedUserID,
		Status:          models.ConnectionStatusPending,
		Direction:       models.ConnectionDirectionOneway,
	}

	if err := h.db.Create(connection).Error; err != nil {
		log.Printf("Error creating connection request: %v", err)
		http.Error(w, "Failed to send connection request", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(buildConnectionResponse(connection, targetUser)); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ConnectionHandler) AcceptRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ConnectedUserID == uuid.Nil {
		http.Error(w, "connected_user_id is required", http.StatusBadRequest)
		return
	}

	var connection models.UserConnection
	err := h.db.Where(
		"user_id = ? AND connected_user_id = ? AND status = ?",
		req.ConnectedUserID, userID, models.ConnectionStatusPending,
	).First(&connection).Error

	if err != nil {
		http.Error(w, "Pending connection request not found", http.StatusNotFound)
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		connection.Status = models.ConnectionStatusAccepted
		connection.Direction = models.ConnectionDirectionMutual
		if err := tx.Save(&connection).Error; err != nil {
			return err
		}

		var reverseConnection models.UserConnection
		err := tx.Where(
			"user_id = ? AND connected_user_id = ?",
			userID, req.ConnectedUserID,
		).First(&reverseConnection).Error

		if err != nil {
			reverse := &models.UserConnection{
				UserID:          userID,
				ConnectedUserID: req.ConnectedUserID,
				Status:          models.ConnectionStatusAccepted,
				Direction:       models.ConnectionDirectionMutual,
			}
			if err := tx.Create(reverse).Error; err != nil {
				return err
			}
		} else {
			reverseConnection.Status = models.ConnectionStatusAccepted
			reverseConnection.Direction = models.ConnectionDirectionMutual
			if err := tx.Save(&reverseConnection).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		log.Printf("Error accepting connection: %v", err)
		http.Error(w, "Failed to accept connection", http.StatusInternalServerError)
		return
	}

	var requester models.User
	h.db.First(&requester, "id = ?", req.ConnectedUserID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(buildConnectionResponse(&connection, requester)); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ConnectionHandler) RejectRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ConnectedUserID == uuid.Nil {
		http.Error(w, "connected_user_id is required", http.StatusBadRequest)
		return
	}

	result := h.db.Where(
		"user_id = ? AND connected_user_id = ? AND status = ?",
		req.ConnectedUserID, userID, models.ConnectionStatusPending,
	).Delete(&models.UserConnection{})

	if result.Error != nil {
		log.Printf("Error rejecting connection: %v", result.Error)
		http.Error(w, "Failed to reject connection", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Pending connection request not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "rejected"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ConnectionHandler) RemoveConnection(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ConnectedUserID == uuid.Nil {
		http.Error(w, "connected_user_id is required", http.StatusBadRequest)
		return
	}

	result := h.db.Where(
		"(user_id = ? AND connected_user_id = ?) OR (user_id = ? AND connected_user_id = ?)",
		userID, req.ConnectedUserID, req.ConnectedUserID, userID,
	).Delete(&models.UserConnection{})

	if result.Error != nil {
		log.Printf("Error removing connection: %v", result.Error)
		http.Error(w, "Failed to remove connection", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "removed"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ConnectionHandler) GetConnections(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	statusFilter := r.URL.Query().Get("status")
	query := h.db.Where("user_id = ?", userID)
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	} else {
		query = query.Where("status = ?", models.ConnectionStatusAccepted)
	}

	var connections []models.UserConnection
	if err := query.Find(&connections).Error; err != nil {
		log.Printf("Error fetching connections: %v", err)
		http.Error(w, "Failed to fetch connections", http.StatusInternalServerError)
		return
	}

	response := h.makeConnectionListResponse(connections)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ConnectionHandler) GetPendingRequests(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var connections []models.UserConnection
	if err := h.db.Where("connected_user_id = ? AND status = ?", userID, models.ConnectionStatusPending).
		Find(&connections).Error; err != nil {
		log.Printf("Error fetching pending requests: %v", err)
		http.Error(w, "Failed to fetch pending requests", http.StatusInternalServerError)
		return
	}

	response := h.makeConnectionListResponse(connections)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ConnectionHandler) GetSentRequests(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var connections []models.UserConnection
	if err := h.db.Where("user_id = ? AND status = ?", userID, models.ConnectionStatusPending).
		Find(&connections).Error; err != nil {
		log.Printf("Error fetching sent requests: %v", err)
		http.Error(w, "Failed to fetch sent requests", http.StatusInternalServerError)
		return
	}

	response := h.makeConnectionListResponse(connections)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ConnectionHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	targetIDStr := r.URL.Query().Get("user_id")
	if targetIDStr == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	targetID, err := uuid.Parse(targetIDStr)
	if err != nil || targetID == uuid.Nil {
		http.Error(w, "Invalid user_id", http.StatusBadRequest)
		return
	}

	var connection models.UserConnection
	err = h.db.Where(
		"(user_id = ? AND connected_user_id = ?) OR (user_id = ? AND connected_user_id = ?)",
		userID, targetID, targetID, userID,
	).First(&connection).Error

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(map[string]string{"status": "none"}); encErr != nil {
			log.Printf("Error encoding response: %v", encErr)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":    string(connection.Status),
		"direction": string(connection.Direction),
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ConnectionHandler) BlockConnection(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ConnectedUserID == uuid.Nil {
		http.Error(w, "connected_user_id is required", http.StatusBadRequest)
		return
	}

	if req.ConnectedUserID == userID {
		http.Error(w, "Cannot block yourself", http.StatusBadRequest)
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		tx.Where(
			"(user_id = ? AND connected_user_id = ?) OR (user_id = ? AND connected_user_id = ?)",
			userID, req.ConnectedUserID, req.ConnectedUserID, userID,
		).Delete(&models.UserConnection{})

		connection := &models.UserConnection{
			UserID:          userID,
			ConnectedUserID: req.ConnectedUserID,
			Status:          models.ConnectionStatusBlocked,
			Direction:       models.ConnectionDirectionOneway,
		}
		return tx.Create(connection).Error
	})

	if err != nil {
		log.Printf("Error blocking connection: %v", err)
		http.Error(w, "Failed to block connection", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "blocked"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func buildConnectionResponse(conn *models.UserConnection, user models.User) ConnectionResponse {
	return ConnectionResponse{
		ID:              conn.ID,
		UserID:          conn.UserID,
		ConnectedUserID: conn.ConnectedUserID,
		Username:        user.Username,
		DisplayName:     user.DisplayName,
		AvatarURL:       user.AvatarURL,
		Status:          string(conn.Status),
		Direction:       string(conn.Direction),
		CreatedAt:       conn.CreatedAt.String(),
		UpdatedAt:       conn.UpdatedAt.String(),
	}
}

func (h *ConnectionHandler) makeConnectionListResponse(connections []models.UserConnection) ConnectionListResponse {
	if len(connections) == 0 {
		return ConnectionListResponse{
			Connections: []ConnectionResponse{},
			Total:       0,
		}
	}

	userIDs := make([]uuid.UUID, 0, len(connections)*2)
	for _, conn := range connections {
		userIDs = append(userIDs, conn.UserID, conn.ConnectedUserID)
	}

	uniqueIDs := make(map[uuid.UUID]bool)
	deduped := make([]uuid.UUID, 0, len(userIDs))
	for _, id := range userIDs {
		if !uniqueIDs[id] {
			uniqueIDs[id] = true
			deduped = append(deduped, id)
		}
	}

	var dbUsers []models.User
	h.db.Where("id IN ?", deduped).Find(&dbUsers)

	userMap := make(map[uuid.UUID]models.User)
	for _, u := range dbUsers {
		userMap[u.ID] = u
	}

	responses := make([]ConnectionResponse, len(connections))
	for i, conn := range connections {
		otherUser := userMap[conn.ConnectedUserID]
		responses[i] = buildConnectionResponse(&conn, otherUser)
	}

	return ConnectionListResponse{
		Connections: responses,
		Total:       len(responses),
	}
}
