package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type StickerPackHandler struct {
	db *database.Database
}

func NewStickerPackHandler(db *database.Database) *StickerPackHandler {
	return &StickerPackHandler{db: db}
}

type CreateStickerPackRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CoverURL    string `json:"cover_url"`
	IsPremium   bool   `json:"is_premium"`
	Price       int    `json:"price"`
	IsGlobal    bool   `json:"is_global"`
}

type StickerPackResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CoverURL    string     `json:"cover_url"`
	IsPremium   bool       `json:"is_premium"`
	Price       int        `json:"price"`
	CreatedBy   *uuid.UUID `json:"created_by"`
	IsGlobal    bool       `json:"is_global"`
	UseCount    int        `json:"use_count"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (h *StickerPackHandler) CreateStickerPack(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateStickerPackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	if len(req.Name) > 100 {
		http.Error(w, "Name too long (max 100 characters)", http.StatusBadRequest)
		return
	}

	pack := &models.StickerPack{
		Name:        req.Name,
		Description: req.Description,
		CoverURL:    req.CoverURL,
		IsPremium:   req.IsPremium,
		Price:       req.Price,
		CreatedBy:   &userID,
		IsGlobal:    req.IsGlobal,
	}

	if err := h.db.Create(pack).Error; err != nil {
		log.Printf("Error creating sticker pack: %v", err)
		http.Error(w, "Failed to create sticker pack", http.StatusInternalServerError)
		return
	}

	response := StickerPackResponse{
		ID:          pack.ID,
		Name:        pack.Name,
		Description: pack.Description,
		CoverURL:    pack.CoverURL,
		IsPremium:   pack.IsPremium,
		Price:       pack.Price,
		CreatedBy:   pack.CreatedBy,
		IsGlobal:    pack.IsGlobal,
		UseCount:    pack.UseCount,
		CreatedAt:   pack.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *StickerPackHandler) GetStickerPacks(w http.ResponseWriter, r *http.Request) {
	var packs []models.StickerPack

	query := h.db.Order("use_count DESC, created_at DESC")
	if r.URL.Query().Get("global") == "true" {
		query = query.Where("is_global = ?", true)
	}
	if r.URL.Query().Get("premium") == "true" {
		query = query.Where("is_premium = ?", true)
	}

	if err := query.Find(&packs).Error; err != nil {
		log.Printf("Error fetching sticker packs: %v", err)
		http.Error(w, "Failed to fetch sticker packs", http.StatusInternalServerError)
		return
	}

	var response []StickerPackResponse
	for _, pack := range packs {
		response = append(response, StickerPackResponse{
			ID:          pack.ID,
			Name:        pack.Name,
			Description: pack.Description,
			CoverURL:    pack.CoverURL,
			IsPremium:   pack.IsPremium,
			Price:       pack.Price,
			CreatedBy:   pack.CreatedBy,
			IsGlobal:    pack.IsGlobal,
			UseCount:    pack.UseCount,
			CreatedAt:   pack.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type RoomRatingHandler struct {
	db *database.Database
}

func NewRoomRatingHandler(db *database.Database) *RoomRatingHandler {
	return &RoomRatingHandler{db: db}
}

type CreateRoomRatingRequest struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

type RoomRatingResponse struct {
	ID        uuid.UUID `json:"id"`
	RoomID    uuid.UUID `json:"room_id"`
	UserID    uuid.UUID `json:"user_id"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *RoomRatingHandler) CreateRating(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	roomIDStr := r.URL.Query().Get("room_id")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room_id", http.StatusBadRequest)
		return
	}

	var req CreateRoomRatingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Rating < 1 || req.Rating > 5 {
		http.Error(w, "Rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	rating := &models.RoomRating{
		RoomID:  roomID,
		UserID:  userID,
		Rating:  req.Rating,
		Comment: req.Comment,
	}

	if err := h.db.Create(rating).Error; err != nil {
		log.Printf("Error creating room rating: %v", err)
		http.Error(w, "Failed to create rating", http.StatusInternalServerError)
		return
	}

	response := RoomRatingResponse{
		ID:        rating.ID,
		RoomID:    rating.RoomID,
		UserID:    rating.UserID,
		Rating:    rating.Rating,
		Comment:   rating.Comment,
		CreatedAt: rating.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *RoomRatingHandler) GetRoomRatings(w http.ResponseWriter, r *http.Request) {
	roomIDStr := r.URL.Query().Get("room_id")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room_id", http.StatusBadRequest)
		return
	}

	var ratings []models.RoomRating
	if err := h.db.Where("room_id = ?", roomID).Order("created_at DESC").Find(&ratings).Error; err != nil {
		log.Printf("Error fetching room ratings: %v", err)
		http.Error(w, "Failed to fetch ratings", http.StatusInternalServerError)
		return
	}

	var avgRating float64
	var total int64
	h.db.Model(&models.RoomRating{}).Where("room_id = ?", roomID).Count(&total)
	if total > 0 {
		var sum int64
		h.db.Model(&models.RoomRating{}).Where("room_id = ?", roomID).Select("COALESCE(SUM(rating), 0)").Scan(&sum)
		avgRating = float64(sum) / float64(total)
	}

	type RatingListResponse struct {
		Ratings []RoomRatingResponse `json:"ratings"`
		Average float64              `json:"average"`
		Total   int64                `json:"total"`
	}

	var responseRatings []RoomRatingResponse
	for _, rating := range ratings {
		responseRatings = append(responseRatings, RoomRatingResponse{
			ID:        rating.ID,
			RoomID:    rating.RoomID,
			UserID:    rating.UserID,
			Rating:    rating.Rating,
			Comment:   rating.Comment,
			CreatedAt: rating.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RatingListResponse{
		Ratings: responseRatings,
		Average: avgRating,
		Total:   total,
	})
}

type UserActivityLogHandler struct {
	db *database.Database
}

func NewUserActivityLogHandler(db *database.Database) *UserActivityLogHandler {
	return &UserActivityLogHandler{db: db}
}

type CreateActivityLogRequest struct {
	ActivityType string                 `json:"activity_type"`
	RoomID       *uuid.UUID             `json:"room_id"`
	TargetType   string                 `json:"target_type"`
	TargetID     *uuid.UUID             `json:"target_id"`
	Metadata     map[string]interface{} `json:"metadata"`
	IPAddress    string                 `json:"ip_address"`
	UserAgent    string                 `json:"user_agent"`
}

type ActivityLogResponse struct {
	ID           uuid.UUID              `json:"id"`
	UserID       uuid.UUID              `json:"user_id"`
	ActivityType string                 `json:"activity_type"`
	RoomID       *uuid.UUID             `json:"room_id"`
	TargetType   string                 `json:"target_type"`
	TargetID     *uuid.UUID             `json:"target_id"`
	Metadata     map[string]interface{} `json:"metadata"`
	IPAddress    string                 `json:"ip_address"`
	UserAgent    string                 `json:"user_agent"`
	SessionID    *uuid.UUID             `json:"session_id"`
	CreatedAt    time.Time              `json:"created_at"`
}

func (h *UserActivityLogHandler) LogActivity(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateActivityLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ActivityType == "" {
		http.Error(w, "Activity type is required", http.StatusBadRequest)
		return
	}

	metadataJSON, _ := json.Marshal(req.Metadata)

	activityLog := &models.UserActivityLog{
		UserID:       userID,
		ActivityType: req.ActivityType,
		RoomID:       req.RoomID,
		TargetType:   req.TargetType,
		TargetID:     req.TargetID,
		Metadata:     string(metadataJSON),
		IPAddress:    req.IPAddress,
		UserAgent:    req.UserAgent,
	}

	if err := h.db.Create(activityLog).Error; err != nil {
		log.Printf("Error creating activity log: %v", err)
		http.Error(w, "Failed to log activity", http.StatusInternalServerError)
		return
	}

	response := ActivityLogResponse{
		ID:           activityLog.ID,
		UserID:       activityLog.UserID,
		ActivityType: activityLog.ActivityType,
		RoomID:       activityLog.RoomID,
		TargetType:   activityLog.TargetType,
		TargetID:     activityLog.TargetID,
		IPAddress:    activityLog.IPAddress,
		UserAgent:    activityLog.UserAgent,
		SessionID:    activityLog.SessionID,
		CreatedAt:    activityLog.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *UserActivityLogHandler) GetUserActivity(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var activityLogs []models.UserActivityLog
	query := h.db.Where("user_id = ?", userID).Order("created_at DESC").Limit(100)

	if activityType := r.URL.Query().Get("type"); activityType != "" {
		query = query.Where("activity_type = ?", activityType)
	}

	if err := query.Find(&activityLogs).Error; err != nil {
		log.Printf("Error fetching activity logs: %v", err)
		http.Error(w, "Failed to fetch activity logs", http.StatusInternalServerError)
		return
	}

	var response []ActivityLogResponse
	for _, activityLog := range activityLogs {
		var metadata map[string]interface{}
		_ = json.Unmarshal([]byte(activityLog.Metadata), &metadata)
		response = append(response, ActivityLogResponse{
			ID:           activityLog.ID,
			UserID:       activityLog.UserID,
			ActivityType: activityLog.ActivityType,
			RoomID:       activityLog.RoomID,
			TargetType:   activityLog.TargetType,
			TargetID:     activityLog.TargetID,
			Metadata:     metadata,
			IPAddress:    activityLog.IPAddress,
			UserAgent:    activityLog.UserAgent,
			SessionID:    activityLog.SessionID,
			CreatedAt:    activityLog.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type RoomMetricHandler struct {
	db *database.Database
}

func NewRoomMetricHandler(db *database.Database) *RoomMetricHandler {
	return &RoomMetricHandler{db: db}
}

type UpdateRoomMetricRequest struct {
	MessageCount      int `json:"message_count"`
	UniqueUsers       int `json:"unique_users"`
	VoiceMinutes      int `json:"voice_minutes"`
	FileUploads       int `json:"file_uploads"`
	AvgResponseTimeMs int `json:"avg_response_time_ms"`
	PeakUsers         int `json:"peak_users"`
}

type RoomMetricResponse struct {
	ID                uuid.UUID `json:"id"`
	RoomID            uuid.UUID `json:"room_id"`
	Date              time.Time `json:"date"`
	MessageCount      int       `json:"message_count"`
	UniqueUsers       int       `json:"unique_users"`
	VoiceMinutes      int       `json:"voice_minutes"`
	FileUploads       int       `json:"file_uploads"`
	AvgResponseTimeMs int       `json:"avg_response_time_ms"`
	PeakUsers         int       `json:"peak_users"`
	CreatedAt         time.Time `json:"created_at"`
}

func (h *RoomMetricHandler) UpdateMetric(w http.ResponseWriter, r *http.Request) {
	roomIDStr := r.URL.Query().Get("room_id")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room_id", http.StatusBadRequest)
		return
	}

	var req UpdateRoomMetricRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	today := time.Now().Truncate(24 * time.Hour)

	var metric models.RoomMetric
	result := h.db.Where("room_id = ? AND date = ?", roomID, today).First(&metric)

	if result.Error != nil {
		metric = models.RoomMetric{
			RoomID:            roomID,
			Date:              today,
			MessageCount:      req.MessageCount,
			UniqueUsers:       req.UniqueUsers,
			VoiceMinutes:      req.VoiceMinutes,
			FileUploads:       req.FileUploads,
			AvgResponseTimeMs: req.AvgResponseTimeMs,
			PeakUsers:         req.PeakUsers,
		}
		if err := h.db.Create(&metric).Error; err != nil {
			log.Printf("Error creating room metric: %v", err)
			http.Error(w, "Failed to create metric", http.StatusInternalServerError)
			return
		}
	} else {
		metric.MessageCount += req.MessageCount
		metric.UniqueUsers = req.UniqueUsers
		metric.VoiceMinutes += req.VoiceMinutes
		metric.FileUploads += req.FileUploads
		metric.AvgResponseTimeMs = req.AvgResponseTimeMs
		if req.PeakUsers > metric.PeakUsers {
			metric.PeakUsers = req.PeakUsers
		}
		if err := h.db.Save(&metric).Error; err != nil {
			log.Printf("Error updating room metric: %v", err)
			http.Error(w, "Failed to update metric", http.StatusInternalServerError)
			return
		}
	}

	response := RoomMetricResponse{
		ID:                metric.ID,
		RoomID:            metric.RoomID,
		Date:              metric.Date,
		MessageCount:      metric.MessageCount,
		UniqueUsers:       metric.UniqueUsers,
		VoiceMinutes:      metric.VoiceMinutes,
		FileUploads:       metric.FileUploads,
		AvgResponseTimeMs: metric.AvgResponseTimeMs,
		PeakUsers:         metric.PeakUsers,
		CreatedAt:         metric.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *RoomMetricHandler) GetRoomMetrics(w http.ResponseWriter, r *http.Request) {
	roomIDStr := r.URL.Query().Get("room_id")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room_id", http.StatusBadRequest)
		return
	}

	var metrics []models.RoomMetric
	if err := h.db.Where("room_id = ?", roomID).Order("date DESC").Limit(30).Find(&metrics).Error; err != nil {
		log.Printf("Error fetching room metrics: %v", err)
		http.Error(w, "Failed to fetch metrics", http.StatusInternalServerError)
		return
	}

	var response []RoomMetricResponse
	for _, m := range metrics {
		response = append(response, RoomMetricResponse{
			ID:                m.ID,
			RoomID:            m.RoomID,
			Date:              m.Date,
			MessageCount:      m.MessageCount,
			UniqueUsers:       m.UniqueUsers,
			VoiceMinutes:      m.VoiceMinutes,
			FileUploads:       m.FileUploads,
			AvgResponseTimeMs: m.AvgResponseTimeMs,
			PeakUsers:         m.PeakUsers,
			CreatedAt:         m.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
