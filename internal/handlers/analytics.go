package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type AnalyticsHandler struct {
	db *database.Database
}

func NewAnalyticsHandler(db *database.Database) *AnalyticsHandler {
	return &AnalyticsHandler{db: db}
}

type AnalyticsOverview struct {
	TotalMessages  int     `json:"total_messages"`
	TotalUsers     int     `json:"total_users"`
	ActiveUsers    int     `json:"active_users"`
	NewUsers       int     `json:"new_users"`
	VoiceMinutes   int     `json:"voice_minutes"`
	MessagesPerDay float64 `json:"messages_per_day"`
}

type DailyActivity struct {
	Date         string `json:"date"`
	MessageCount int    `json:"message_count"`
	UserCount    int    `json:"user_count"`
}

type ChannelStats struct {
	RoomID       uuid.UUID `json:"room_id"`
	RoomName     string    `json:"room_name"`
	MessageCount int       `json:"message_count"`
	UserCount    int       `json:"user_count"`
}

type HourlyStats struct {
	Hour         int `json:"hour"`
	MessageCount int `json:"message_count"`
	UserCount    int `json:"user_count"`
}

func (h *AnalyticsHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := 30
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
		days = d
	}

	since := time.Now().AddDate(0, 0, -days)

	var totalMessages int64
	h.db.Model(&models.Message{}).Where("created_at >= ?", since).Count(&totalMessages)

	var totalUsers int64
	h.db.Model(&models.User{}).Count(&totalUsers)

	var activeUsers int64
	h.db.Model(&models.Message{}).Where("created_at >= ?", since).Distinct("user_id").Count(&activeUsers)

	var newUsers int64
	h.db.Model(&models.User{}).Where("created_at >= ?", since).Count(&newUsers)

	voiceMinutes := 0

	messagesPerDay := 0.0
	if days > 0 {
		messagesPerDay = float64(totalMessages) / float64(days)
	}

	overview := AnalyticsOverview{
		TotalMessages:  int(totalMessages),
		TotalUsers:     int(totalUsers),
		ActiveUsers:    int(activeUsers),
		NewUsers:       int(newUsers),
		VoiceMinutes:   voiceMinutes,
		MessagesPerDay: messagesPerDay,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(overview); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AnalyticsHandler) GetDailyActivity(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := 30
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
		days = d
	}

	since := time.Now().AddDate(0, 0, -days)

	type dailyCount struct {
		Date         string `json:"date"`
		MessageCount int    `json:"message_count"`
		UserCount    int    `json:"user_count"`
	}

	var results []dailyCount
	err := h.db.Model(&models.Message{}).
		Select("date(created_at) as date, COUNT(*) as message_count, COUNT(DISTINCT user_id) as user_count").
		Where("created_at >= ?", since).
		Group("date(created_at)").
		Order("date ASC").
		Scan(&results).Error

	if err != nil {
		log.Printf("Error fetching daily activity: %v", err)
		http.Error(w, "Failed to fetch daily activity", http.StatusInternalServerError)
		return
	}

	var response []DailyActivity
	for _, r := range results {
		response = append(response, DailyActivity(r))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AnalyticsHandler) GetChannelStats(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := 30
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
		days = d
	}

	since := time.Now().AddDate(0, 0, -days)

	type channelStats struct {
		RoomID       uuid.UUID `json:"room_id"`
		RoomName     string    `json:"room_name"`
		MessageCount int       `json:"message_count"`
		UserCount    int       `json:"user_count"`
	}

	var results []channelStats
	err := h.db.Model(&models.Message{}).
		Select("messages.room_id, rooms.name as room_name, COUNT(*) as message_count, COUNT(DISTINCT messages.user_id) as user_count").
		Joins("JOIN rooms ON messages.room_id = rooms.id").
		Where("messages.created_at >= ?", since).
		Group("messages.room_id, rooms.name").
		Order("message_count DESC").
		Limit(10).
		Scan(&results).Error

	if err != nil {
		log.Printf("Error fetching channel stats: %v", err)
		http.Error(w, "Failed to fetch channel stats", http.StatusInternalServerError)
		return
	}

	var response []ChannelStats
	for _, r := range results {
		response = append(response, ChannelStats(r))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AnalyticsHandler) GetHourlyActivity(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := 30
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
		days = d
	}

	since := time.Now().AddDate(0, 0, -days)

	type hourlyData struct {
		Hour         int `json:"hour"`
		MessageCount int `json:"message_count"`
		UserCount    int `json:"user_count"`
	}

	var results []hourlyData
	err := h.db.Model(&models.Message{}).
		Select("CAST(strftime('%H', created_at) AS INTEGER) as hour, COUNT(*) as message_count, COUNT(DISTINCT user_id) as user_count").
		Where("created_at >= ?", since).
		Group("CAST(strftime('%H', created_at) AS INTEGER)").
		Order("hour ASC").
		Scan(&results).Error

	if err != nil {
		log.Printf("Error fetching hourly activity: %v", err)
		http.Error(w, "Failed to fetch hourly activity", http.StatusInternalServerError)
		return
	}

	hourlyMap := make(map[int]HourlyStats)
	for _, r := range results {
		hourlyMap[r.Hour] = HourlyStats(r)
	}

	var response []HourlyStats
	for h := 0; h < 24; h++ {
		if stats, ok := hourlyMap[h]; ok {
			response = append(response, stats)
		} else {
			response = append(response, HourlyStats{Hour: h, MessageCount: 0, UserCount: 0})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AnalyticsHandler) GetTopUsers(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := 30
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 365 {
		days = d
	}

	since := time.Now().AddDate(0, 0, -days)
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	type topUser struct {
		UserID       uuid.UUID `json:"user_id"`
		Username     string    `json:"username"`
		AvatarURL    string    `json:"avatar_url"`
		MessageCount int       `json:"message_count"`
	}

	var results []topUser
	err := h.db.Model(&models.Message{}).
		Select("messages.user_id, users.username, users.avatar_url, COUNT(*) as message_count").
		Joins("JOIN users ON messages.user_id = users.id").
		Where("messages.created_at >= ?", since).
		Group("messages.user_id, users.username, users.avatar_url").
		Order("message_count DESC").
		Limit(limit).
		Scan(&results).Error

	if err != nil {
		log.Printf("Error fetching top users: %v", err)
		http.Error(w, "Failed to fetch top users", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AnalyticsHandler) RecordActivity(userID uuid.UUID, activityType string) {
	now := time.Now()
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	switch activityType {
	case "message":
		var daily models.DailyAnalytics
		err := h.db.FirstOrCreate(&daily, models.DailyAnalytics{Date: date}).Error
		if err == nil {
			daily.TotalMessages++
			h.db.Save(&daily)
		}

		var hourly models.HourlyActivity
		err = h.db.FirstOrCreate(&hourly, models.HourlyActivity{Date: date, Hour: now.Hour()}).Error
		if err == nil {
			hourly.MessageCount++
			h.db.Save(&hourly)
		}

		var channel models.ChannelActivity
		err = h.db.FirstOrCreate(&channel, models.ChannelActivity{Date: date}).Error
		if err == nil {
			channel.MessageCount++
			h.db.Save(&channel)
		}
	}
}

func (h *AnalyticsHandler) RequireAdmin(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if !user.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}
}
