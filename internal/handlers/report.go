package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type ReportHandler struct {
	db *database.Database
}

func NewReportHandler(db *database.Database) *ReportHandler {
	return &ReportHandler{db: db}
}

type CreateReportRequest struct {
	ReportedID  uuid.UUID           `json:"reported_id"`
	RoomID      uuid.UUID           `json:"room_id"`
	MessageID   *uuid.UUID          `json:"message_id"`
	Reason      models.ReportReason `json:"reason"`
	Description string              `json:"description"`
}

type ReviewReportRequest struct {
	Status      models.ReportStatus `json:"status"`
	ReviewNotes string              `json:"review_notes"`
}

func (h *ReportHandler) CreateReport(w http.ResponseWriter, r *http.Request) {
	var req CreateReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	reporterID := middleware.GetUserID(r)

	if req.ReportedID == uuid.Nil {
		http.Error(w, "reported_id is required", http.StatusBadRequest)
		return
	}

	if req.RoomID == uuid.Nil {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	if req.ReportedID == reporterID {
		http.Error(w, "Cannot report yourself", http.StatusBadRequest)
		return
	}

	var reported models.User
	if err := h.db.First(&reported, req.ReportedID).Error; err != nil {
		http.Error(w, "Reported user not found", http.StatusNotFound)
		return
	}

	var room models.Room
	if err := h.db.First(&room, req.RoomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	report := models.Report{
		ReporterID:  reporterID,
		ReportedID:  req.ReportedID,
		RoomID:      req.RoomID,
		MessageID:   req.MessageID,
		Reason:      req.Reason,
		Description: req.Description,
		Status:      models.ReportStatusPending,
	}

	if err := h.db.Create(&report).Error; err != nil {
		http.Error(w, "Failed to create report", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"message": "report submitted successfully", "report": report})
}

func (h *ReportHandler) GetReports(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	status := r.URL.Query().Get("status")

	query := h.db.Model(&models.Report{})

	if status != "" {
		query = query.Where("status = ?", status)
	}

	var reports []models.Report
	if err := query.Order("created_at DESC").Find(&reports).Error; err != nil {
		http.Error(w, "Failed to fetch reports", http.StatusInternalServerError)
		return
	}

	type ReportResponse struct {
		ID          uuid.UUID           `json:"id"`
		ReporterID  uuid.UUID           `json:"reporter_id"`
		ReportedID  uuid.UUID           `json:"reported_id"`
		RoomID      uuid.UUID           `json:"room_id"`
		MessageID   *uuid.UUID          `json:"message_id"`
		Reason      models.ReportReason `json:"reason"`
		Description string              `json:"description"`
		Status      models.ReportStatus `json:"status"`
		ReviewedBy  *uuid.UUID          `json:"reviewed_by"`
		ReviewNotes string              `json:"review_notes"`
		CreatedAt   string              `json:"created_at"`
		UpdatedAt   string              `json:"updated_at"`
	}

	response := make([]ReportResponse, len(reports))
	for i, rep := range reports {
		response[i] = ReportResponse{
			ID:          rep.ID,
			ReporterID:  rep.ReporterID,
			ReportedID:  rep.ReportedID,
			RoomID:      rep.RoomID,
			MessageID:   rep.MessageID,
			Reason:      rep.Reason,
			Description: rep.Description,
			Status:      rep.Status,
			ReviewedBy:  rep.ReviewedBy,
			ReviewNotes: rep.ReviewNotes,
			CreatedAt:   rep.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   rep.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"reports": response})
}

func (h *ReportHandler) GetReport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	reportID := r.URL.Query().Get("id")
	if reportID == "" {
		http.Error(w, "report_id is required", http.StatusBadRequest)
		return
	}

	reportUUID, err := uuid.Parse(reportID)
	if err != nil {
		http.Error(w, "Invalid report_id", http.StatusBadRequest)
		return
	}

	var report models.Report
	if err := h.db.First(&report, reportUUID).Error; err != nil {
		http.Error(w, "Report not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"report": report})
}

func (h *ReportHandler) ReviewReport(w http.ResponseWriter, r *http.Request) {
	reportID := r.URL.Query().Get("id")
	if reportID == "" {
		http.Error(w, "report_id is required", http.StatusBadRequest)
		return
	}

	reportUUID, err := uuid.Parse(reportID)
	if err != nil {
		http.Error(w, "Invalid report_id", http.StatusBadRequest)
		return
	}

	var req ReviewReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r)

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	var report models.Report
	if err := h.db.First(&report, reportUUID).Error; err != nil {
		http.Error(w, "Report not found", http.StatusNotFound)
		return
	}

	report.Status = req.Status
	report.ReviewedBy = &userID
	report.ReviewNotes = req.ReviewNotes

	if err := h.db.Save(&report).Error; err != nil {
		http.Error(w, "Failed to update report", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"message": "report reviewed successfully", "report": report})
}

func (h *ReportHandler) DeleteReport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	reportID := r.URL.Query().Get("id")
	if reportID == "" {
		http.Error(w, "report_id is required", http.StatusBadRequest)
		return
	}

	reportUUID, err := uuid.Parse(reportID)
	if err != nil {
		http.Error(w, "Invalid report_id", http.StatusBadRequest)
		return
	}

	var report models.Report
	if err := h.db.First(&report, reportUUID).Error; err != nil {
		http.Error(w, "Report not found", http.StatusNotFound)
		return
	}

	if err := h.db.Delete(&report).Error; err != nil {
		http.Error(w, "Failed to delete report", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"message": "report deleted successfully"})
}

func (h *ReportHandler) GetMyReports(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var reports []models.Report
	if err := h.db.Where("reporter_id = ?", userID).Order("created_at DESC").Find(&reports).Error; err != nil {
		http.Error(w, "Failed to fetch reports", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"reports": reports})
}
