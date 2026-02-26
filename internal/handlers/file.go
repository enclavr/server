package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type FileHandler struct {
	db          *database.Database
	uploadDir   string
	maxFileSize int64
}

func NewFileHandler(db *database.Database, uploadDir string, maxFileSizeMB int) *FileHandler {
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	if maxFileSizeMB == 0 {
		maxFileSizeMB = 10
	}

	handler := &FileHandler{
		db:          db,
		uploadDir:   uploadDir,
		maxFileSize: int64(maxFileSizeMB) * 1024 * 1024,
	}

	if err := os.MkdirAll(handler.uploadDir, 0755); err != nil {
		log.Printf("Warning: Could not create upload directory: %v", err)
	}

	return handler
}

type UploadResponse struct {
	ID          string `json:"id"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
}

func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var settings models.ServerSettings
	if err := h.db.First(&settings).Error; err != nil || !settings.EnableFileUploads {
		http.Error(w, "File uploads are disabled", http.StatusForbidden)
		return
	}

	if r.ContentLength > h.maxFileSize || r.ContentLength > int64(settings.MaxUploadSizeMB)*1024*1024 {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	if header.Size > h.maxFileSize || header.Size > int64(settings.MaxUploadSizeMB)*1024*1024 {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	roomIDStr := r.FormValue("room_id")
	if roomIDStr == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	result := h.db.Where("user_id = ? AND room_id = ?", userID, roomID).First(&userRoom)
	if result.Error != nil {
		http.Error(w, "Not in room", http.StatusForbidden)
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowedExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
		".mp4":  true,
		".webm": true,
		".mp3":  true,
		".wav":  true,
		".ogg":  true,
		".pdf":  true,
		".txt":  true,
		".doc":  true,
		".docx": true,
	}

	if !allowedExts[ext] {
		http.Error(w, "File type not allowed", http.StatusBadRequest)
		return
	}

	fileID := uuid.New()
	datePath := time.Now().Format("2006/01/02")
	storageDir := filepath.Join(h.uploadDir, roomID.String(), datePath)
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Printf("Error creating storage directory: %v", err)
		http.Error(w, "Failed to create storage", http.StatusInternalServerError)
		return
	}

	storageKey := filepath.Join(roomID.String(), datePath, fileID.String()+ext)
	storagePath := filepath.Join(h.uploadDir, storageKey)

	dst, err := os.Create(storagePath)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, file); err != nil {
		log.Printf("Error copying file: %v", err)
		if rmErr := os.Remove(storagePath); rmErr != nil {
			log.Printf("Error removing file: %v", rmErr)
		}
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	fileRecord := models.File{
		ID:          fileID,
		UserID:      userID,
		RoomID:      roomID,
		FileName:    header.Filename,
		FileSize:    header.Size,
		ContentType: header.Header.Get("Content-Type"),
		StorageKey:  storageKey,
		URL:         fmt.Sprintf("/api/files/%s", storageKey),
	}

	if err := h.db.Create(&fileRecord).Error; err != nil {
		log.Printf("Error creating file record: %v", err)
		if rmErr := os.Remove(storagePath); rmErr != nil {
			log.Printf("Error removing file: %v", rmErr)
		}
		http.Error(w, "Failed to save file metadata", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(UploadResponse{
		ID:          fileID.String(),
		FileName:    fileRecord.FileName,
		FileSize:    fileRecord.FileSize,
		ContentType: fileRecord.ContentType,
		URL:         fileRecord.URL,
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *FileHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/files/"), "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	storageKey := strings.Join(parts, "/")
	filePath := filepath.Join(h.uploadDir, storageKey)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, filePath)
}

func (h *FileHandler) GetRoomFiles(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	result := h.db.Where("user_id = ? AND room_id = ?", userID, roomID).First(&userRoom)
	if result.Error != nil {
		http.Error(w, "Not in room", http.StatusForbidden)
		return
	}

	var files []models.File
	if err := h.db.Where("room_id = ? AND is_deleted = ?", roomID, false).Order("created_at DESC").Limit(50).Find(&files).Error; err != nil {
		log.Printf("Error fetching files: %v", err)
		http.Error(w, "Failed to fetch files", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(files); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	fileIDStr := r.URL.Query().Get("file_id")
	if fileIDStr == "" {
		http.Error(w, "File ID is required", http.StatusBadRequest)
		return
	}

	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		http.Error(w, "Invalid file ID", http.StatusBadRequest)
		return
	}

	var file models.File
	if err := h.db.First(&file, fileID).Error; err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if file.UserID != userID {
		var userRoom models.UserRoom
		result := h.db.Where("user_id = ? AND room_id = ? AND role IN ?", userID, file.RoomID, []string{"admin", "owner"}).First(&userRoom)
		if result.Error != nil {
			http.Error(w, "Not authorized", http.StatusForbidden)
			return
		}
	}

	file.IsDeleted = true
	if err := h.db.Save(&file).Error; err != nil {
		log.Printf("Error deleting file: %v", err)
		http.Error(w, "Failed to delete file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *FileHandler) UploadToWebhook(url, fileField, fileName, fileContentType string, fileContent []byte) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		return err
	}

	if _, err := part.Write(fileContent); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
