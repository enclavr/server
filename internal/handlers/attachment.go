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
	"golang.org/x/crypto/bcrypt"
)

type AttachmentHandler struct {
	db *database.Database
}

func NewAttachmentHandler(db *database.Database) *AttachmentHandler {
	return &AttachmentHandler{db: db}
}

type CreateAttachmentRequest struct {
	MessageID    uuid.UUID `json:"message_id"`
	FileID       uuid.UUID `json:"file_id"`
	FileName     string    `json:"file_name"`
	FileSize     int64     `json:"file_size"`
	ContentType  string    `json:"content_type"`
	ThumbnailURL string    `json:"thumbnail_url"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	Duration     int       `json:"duration"`
	AltText      string    `json:"alt_text"`
	IsVoiceMemo  bool      `json:"is_voice_memo"`
	WaveformData string    `json:"waveform_data"`
	Metadata     string    `json:"metadata"`
}

type AttachmentResponse struct {
	ID           uuid.UUID `json:"id"`
	MessageID    uuid.UUID `json:"message_id"`
	FileID       uuid.UUID `json:"file_id"`
	UserID       uuid.UUID `json:"user_id"`
	FileName     string    `json:"file_name"`
	FileSize     int64     `json:"file_size"`
	ContentType  string    `json:"content_type"`
	ThumbnailURL string    `json:"thumbnail_url"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	Duration     int       `json:"duration"`
	AltText      string    `json:"alt_text"`
	IsVoiceMemo  bool      `json:"is_voice_memo"`
	WaveformData string    `json:"waveform_data"`
	Metadata     string    `json:"metadata"`
	CreatedAt    time.Time `json:"created_at"`
}

func (h *AttachmentHandler) CreateAttachment(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateAttachmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MessageID == uuid.Nil {
		http.Error(w, "Message ID is required", http.StatusBadRequest)
		return
	}

	if req.FileID == uuid.Nil {
		http.Error(w, "File ID is required", http.StatusBadRequest)
		return
	}

	var message models.Message
	if err := h.db.First(&message, "id = ?", req.MessageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if message.UserID != userID {
		var userRoom models.UserRoom
		if err := h.db.Where("user_id = ? AND room_id = ? AND role IN ?", userID, message.RoomID, []string{"admin", "owner"}).First(&userRoom).Error; err != nil {
			http.Error(w, "Not authorized to add attachments to this message", http.StatusForbidden)
			return
		}
	}

	attachment := models.Attachment{
		MessageID:    req.MessageID,
		FileID:       req.FileID,
		UserID:       userID,
		FileName:     req.FileName,
		FileSize:     req.FileSize,
		ContentType:  req.ContentType,
		ThumbnailURL: req.ThumbnailURL,
		Width:        req.Width,
		Height:       req.Height,
		Duration:     req.Duration,
		AltText:      req.AltText,
		IsVoiceMemo:  req.IsVoiceMemo,
		WaveformData: req.WaveformData,
		Metadata:     req.Metadata,
	}

	if err := h.db.Create(&attachment).Error; err != nil {
		log.Printf("Error creating attachment: %v", err)
		http.Error(w, "Failed to create attachment", http.StatusInternalServerError)
		return
	}

	h.sendAttachmentResponse(w, &attachment, http.StatusCreated)
}

func (h *AttachmentHandler) GetAttachment(w http.ResponseWriter, r *http.Request) {
	attachmentIDStr := r.URL.Query().Get("id")
	if attachmentIDStr == "" {
		http.Error(w, "Attachment ID is required", http.StatusBadRequest)
		return
	}

	attachmentID, err := uuid.Parse(attachmentIDStr)
	if err != nil {
		http.Error(w, "Invalid attachment ID", http.StatusBadRequest)
		return
	}

	var attachment models.Attachment
	if err := h.db.First(&attachment, "id = ?", attachmentID).Error; err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	h.sendAttachmentResponse(w, &attachment, http.StatusOK)
}

func (h *AttachmentHandler) GetMessageAttachments(w http.ResponseWriter, r *http.Request) {
	messageIDStr := r.URL.Query().Get("message_id")
	if messageIDStr == "" {
		http.Error(w, "Message ID is required", http.StatusBadRequest)
		return
	}

	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	var attachments []models.Attachment
	if err := h.db.Where("message_id = ?", messageID).Order("created_at ASC").Find(&attachments).Error; err != nil {
		log.Printf("Error fetching attachments: %v", err)
		http.Error(w, "Failed to fetch attachments", http.StatusInternalServerError)
		return
	}

	responses := make([]AttachmentResponse, len(attachments))
	for i, att := range attachments {
		responses[i] = h.attachmentToResponse(&att)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AttachmentHandler) UpdateAttachment(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		ID           uuid.UUID `json:"id"`
		ThumbnailURL string    `json:"thumbnail_url"`
		AltText      string    `json:"alt_text"`
		Duration     *int      `json:"duration"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var attachment models.Attachment
	if err := h.db.First(&attachment, "id = ?", req.ID).Error; err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	if attachment.UserID != userID {
		http.Error(w, "Not authorized to update this attachment", http.StatusForbidden)
		return
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.ThumbnailURL != "" {
		updates["thumbnail_url"] = req.ThumbnailURL
	}
	if req.AltText != "" {
		updates["alt_text"] = req.AltText
	}
	if req.Duration != nil {
		updates["duration"] = *req.Duration
	}

	if err := h.db.Model(&attachment).Updates(updates).Error; err != nil {
		log.Printf("Error updating attachment: %v", err)
		http.Error(w, "Failed to update attachment", http.StatusInternalServerError)
		return
	}

	h.sendAttachmentResponse(w, &attachment, http.StatusOK)
}

func (h *AttachmentHandler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	attachmentIDStr := r.URL.Query().Get("id")
	if attachmentIDStr == "" {
		http.Error(w, "Attachment ID is required", http.StatusBadRequest)
		return
	}

	attachmentID, err := uuid.Parse(attachmentIDStr)
	if err != nil {
		http.Error(w, "Invalid attachment ID", http.StatusBadRequest)
		return
	}

	var attachment models.Attachment
	if err := h.db.First(&attachment, "id = ?", attachmentID).Error; err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	if attachment.UserID != userID {
		var message models.Message
		if err := h.db.First(&message, "id = ?", attachment.MessageID).Error; err != nil {
			http.Error(w, "Message not found", http.StatusNotFound)
			return
		}

		var userRoom models.UserRoom
		if err := h.db.Where("user_id = ? AND room_id = ? AND role IN ?", userID, message.RoomID, []string{"admin", "owner"}).First(&userRoom).Error; err != nil {
			http.Error(w, "Not authorized to delete this attachment", http.StatusForbidden)
			return
		}
	}

	if err := h.db.Delete(&attachment).Error; err != nil {
		log.Printf("Error deleting attachment: %v", err)
		http.Error(w, "Failed to delete attachment", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AttachmentHandler) attachmentToResponse(attachment *models.Attachment) AttachmentResponse {
	return AttachmentResponse{
		ID:           attachment.ID,
		MessageID:    attachment.MessageID,
		FileID:       attachment.FileID,
		UserID:       attachment.UserID,
		FileName:     attachment.FileName,
		FileSize:     attachment.FileSize,
		ContentType:  attachment.ContentType,
		ThumbnailURL: attachment.ThumbnailURL,
		Width:        attachment.Width,
		Height:       attachment.Height,
		Duration:     attachment.Duration,
		AltText:      attachment.AltText,
		IsVoiceMemo:  attachment.IsVoiceMemo,
		WaveformData: attachment.WaveformData,
		Metadata:     attachment.Metadata,
		CreatedAt:    attachment.CreatedAt,
	}
}

func (h *AttachmentHandler) sendAttachmentResponse(w http.ResponseWriter, attachment *models.Attachment, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(h.attachmentToResponse(attachment)); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

type ShareAttachmentRequest struct {
	AttachmentID uuid.UUID  `json:"attachment_id"`
	Password     string     `json:"password"`
	ExpiresAt    *time.Time `json:"expires_at"`
	MaxDownloads int        `json:"max_downloads"`
}

type ShareAttachmentResponse struct {
	ID            uuid.UUID  `json:"id"`
	AttachmentID  uuid.UUID  `json:"attachment_id"`
	ShareURL      string     `json:"share_url"`
	ExpiresAt     *time.Time `json:"expires_at"`
	MaxDownloads  int        `json:"max_downloads"`
	DownloadCount int        `json:"download_count"`
	ViewCount     int        `json:"view_count"`
	IsActive      bool       `json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (h *AttachmentHandler) ShareAttachment(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ShareAttachmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var attachment models.Attachment
	if err := h.db.First(&attachment, "id = ?", req.AttachmentID).Error; err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	if attachment.UserID != userID {
		http.Error(w, "Not authorized to share this attachment", http.StatusForbidden)
		return
	}

	shareCode := uuid.New().String()[:12]
	shareURL := "/share/" + shareCode

	attachment.IsShared = true
	attachment.ShareCount++

	share := models.AttachmentShare{
		AttachmentID:  req.AttachmentID,
		SharedBy:      userID,
		ShareURL:      shareURL,
		ExpiresAt:     req.ExpiresAt,
		MaxDownloads:  req.MaxDownloads,
		DownloadCount: 0,
		ViewCount:     0,
		IsActive:      true,
	}

	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to hash password", http.StatusInternalServerError)
			return
		}
		share.Password = string(hashedPassword)
	}

	if err := h.db.Create(&share).Error; err != nil {
		log.Printf("Error creating share: %v", err)
		http.Error(w, "Failed to create share", http.StatusInternalServerError)
		return
	}

	if err := h.db.Save(&attachment).Error; err != nil {
		log.Printf("Error updating attachment: %v", err)
	}

	response := ShareAttachmentResponse{
		ID:            share.ID,
		AttachmentID:  share.AttachmentID,
		ShareURL:      shareURL,
		ExpiresAt:     share.ExpiresAt,
		MaxDownloads:  share.MaxDownloads,
		DownloadCount: share.DownloadCount,
		ViewCount:     share.ViewCount,
		IsActive:      share.IsActive,
		CreatedAt:     share.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AttachmentHandler) GetAttachmentShares(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	attachmentIDStr := r.URL.Query().Get("attachment_id")
	if attachmentIDStr == "" {
		http.Error(w, "Attachment ID is required", http.StatusBadRequest)
		return
	}

	attachmentID, err := uuid.Parse(attachmentIDStr)
	if err != nil {
		http.Error(w, "Invalid attachment ID", http.StatusBadRequest)
		return
	}

	var attachment models.Attachment
	if err := h.db.First(&attachment, "id = ?", attachmentID).Error; err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	if attachment.UserID != userID {
		http.Error(w, "Not authorized to view shares", http.StatusForbidden)
		return
	}

	var shares []models.AttachmentShare
	if err := h.db.Where("attachment_id = ?", attachmentID).Order("created_at DESC").Find(&shares).Error; err != nil {
		log.Printf("Error fetching shares: %v", err)
		http.Error(w, "Failed to fetch shares", http.StatusInternalServerError)
		return
	}

	responses := make([]ShareAttachmentResponse, len(shares))
	for i, share := range shares {
		responses[i] = ShareAttachmentResponse{
			ID:            share.ID,
			AttachmentID:  share.AttachmentID,
			ShareURL:      share.ShareURL,
			ExpiresAt:     share.ExpiresAt,
			MaxDownloads:  share.MaxDownloads,
			DownloadCount: share.DownloadCount,
			ViewCount:     share.ViewCount,
			IsActive:      share.IsActive,
			CreatedAt:     share.CreatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AttachmentHandler) GetSharedAttachment(w http.ResponseWriter, r *http.Request) {
	shareCode := r.URL.Query().Get("code")
	if shareCode == "" {
		http.Error(w, "Share code is required", http.StatusBadRequest)
		return
	}

	password := r.URL.Query().Get("password")

	var share models.AttachmentShare
	if err := h.db.Where("share_url = ? AND is_active = ?", "/share/"+shareCode, true).First(&share).Error; err != nil {
		http.Error(w, "Share not found", http.StatusNotFound)
		return
	}

	if share.ExpiresAt != nil && time.Now().After(*share.ExpiresAt) {
		http.Error(w, "Share has expired", http.StatusGone)
		return
	}

	if share.MaxDownloads > 0 && share.DownloadCount >= share.MaxDownloads {
		http.Error(w, "Download limit reached", http.StatusForbidden)
		return
	}

	if share.Password != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(share.Password), []byte(password)); err != nil {
			http.Error(w, "Invalid password", http.StatusUnauthorized)
			return
		}
	}

	share.ViewCount++
	if err := h.db.Save(&share).Error; err != nil {
		log.Printf("Error updating view count: %v", err)
	}

	var attachment models.Attachment
	if err := h.db.First(&attachment, "id = ?", share.AttachmentID).Error; err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"attachment": h.attachmentToResponse(&attachment),
		"share_url":  share.ShareURL,
		"view_count": share.ViewCount,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AttachmentHandler) DeleteShare(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	shareIDStr := r.URL.Query().Get("id")
	if shareIDStr == "" {
		http.Error(w, "Share ID is required", http.StatusBadRequest)
		return
	}

	shareID, err := uuid.Parse(shareIDStr)
	if err != nil {
		http.Error(w, "Invalid share ID", http.StatusBadRequest)
		return
	}

	var share models.AttachmentShare
	if err := h.db.First(&share, "id = ?", shareID).Error; err != nil {
		http.Error(w, "Share not found", http.StatusNotFound)
		return
	}

	var attachment models.Attachment
	if err := h.db.First(&attachment, "id = ?", share.AttachmentID).Error; err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	if attachment.UserID != userID {
		http.Error(w, "Not authorized to delete this share", http.StatusForbidden)
		return
	}

	share.IsActive = false
	if err := h.db.Save(&share).Error; err != nil {
		log.Printf("Error deleting share: %v", err)
		http.Error(w, "Failed to delete share", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
