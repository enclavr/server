package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
)

func setupReportHandlerTest(t *testing.T) (*ReportHandler, *database.Database, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Report{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	testDB := &database.Database{DB: db}
	handler := NewReportHandler(testDB)

	reporterID := uuid.New()
	reportedID := uuid.New()
	roomID := uuid.New()
	adminID := uuid.New()

	reporter := models.User{ID: reporterID, Username: "reporter", Email: "reporter@test.com"}
	reported := models.User{ID: reportedID, Username: "reported", Email: "reported@test.com"}
	admin := models.User{ID: adminID, Username: "admin", Email: "admin@test.com", IsAdmin: true}
	room := models.Room{ID: roomID, Name: "Test Room"}

	db.Create(&reporter)
	db.Create(&reported)
	db.Create(&admin)
	db.Create(&room)

	return handler, testDB, reporterID, reportedID, roomID, adminID
}

func TestReportHandler_CreateReport(t *testing.T) {
	handler, _, reporterID, reportedID, roomID, _ := setupReportHandlerTest(t)

	tests := []struct {
		name           string
		body           CreateReportRequest
		expectedStatus int
	}{
		{
			name: "valid report creation",
			body: CreateReportRequest{
				ReportedID:  reportedID,
				RoomID:      roomID,
				Reason:      models.ReportReasonHarassment,
				Description: "Test description",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing reported_id",
			body: CreateReportRequest{
				RoomID:      roomID,
				Reason:      models.ReportReasonSpam,
				Description: "Test description",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing room_id",
			body: CreateReportRequest{
				ReportedID:  reportedID,
				Reason:      models.ReportReasonSpam,
				Description: "Test description",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "reported user not found",
			body: CreateReportRequest{
				ReportedID:  uuid.New(),
				RoomID:      roomID,
				Reason:      models.ReportReasonSpam,
				Description: "Test description",
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "room not found",
			body: CreateReportRequest{
				ReportedID:  reportedID,
				RoomID:      uuid.New(),
				Reason:      models.ReportReasonSpam,
				Description: "Test description",
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/report/create", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), reporterID))
			w := httptest.NewRecorder()

			handler.CreateReport(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestReportHandler_CreateReport_CannotReportSelf(t *testing.T) {
	handler, _, reporterID, _, roomID, _ := setupReportHandlerTest(t)

	body, _ := json.Marshal(CreateReportRequest{
		ReportedID:  reporterID,
		RoomID:      roomID,
		Reason:      models.ReportReasonSpam,
		Description: "Test",
	})
	req := httptest.NewRequest(http.MethodPost, "/report/create", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), reporterID))
	w := httptest.NewRecorder()

	handler.CreateReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestReportHandler_GetReports(t *testing.T) {
	handler, db, reporterID, reportedID, roomID, adminID := setupReportHandlerTest(t)

	report := models.Report{
		ID:          uuid.New(),
		ReporterID:  reporterID,
		ReportedID:  reportedID,
		RoomID:      roomID,
		Reason:      models.ReportReasonSpam,
		Description: "Test report",
		Status:      models.ReportStatusPending,
	}
	db.Create(&report)

	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.GetReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestReportHandler_GetReports_WithStatusFilter(t *testing.T) {
	handler, db, reporterID, reportedID, roomID, adminID := setupReportHandlerTest(t)

	report := models.Report{
		ID:          uuid.New(),
		ReporterID:  reporterID,
		ReportedID:  reportedID,
		RoomID:      roomID,
		Reason:      models.ReportReasonSpam,
		Description: "Test report",
		Status:      models.ReportStatusResolved,
	}
	db.Create(&report)

	req := httptest.NewRequest(http.MethodGet, "/reports?status=resolved", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.GetReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestReportHandler_GetReport(t *testing.T) {
	handler, db, reporterID, reportedID, roomID, adminID := setupReportHandlerTest(t)

	report := models.Report{
		ID:          uuid.New(),
		ReporterID:  reporterID,
		ReportedID:  reportedID,
		RoomID:      roomID,
		Reason:      models.ReportReasonSpam,
		Description: "Test report",
		Status:      models.ReportStatusPending,
	}
	db.Create(&report)

	req := httptest.NewRequest(http.MethodGet, "/report?id="+report.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.GetReport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestReportHandler_GetReport_MissingID(t *testing.T) {
	handler, _, _, _, _, adminID := setupReportHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/report", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.GetReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestReportHandler_GetReport_InvalidID(t *testing.T) {
	handler, _, _, _, _, adminID := setupReportHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/report?id=invalid", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.GetReport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestReportHandler_GetReport_NotFound(t *testing.T) {
	handler, _, _, _, _, adminID := setupReportHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/report?id="+uuid.New().String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.GetReport(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestReportHandler_ReviewReport(t *testing.T) {
	handler, db, reporterID, _, roomID, adminID := setupReportHandlerTest(t)

	report := models.Report{
		ID:          uuid.New(),
		ReporterID:  reporterID,
		ReportedID:  uuid.New(),
		RoomID:      roomID,
		Reason:      models.ReportReasonSpam,
		Description: "Test report",
		Status:      models.ReportStatusPending,
	}
	db.Create(&report)

	reviewReq := ReviewReportRequest{
		Status:      models.ReportStatusResolved,
		ReviewNotes: "Reviewed and resolved",
	}
	body, _ := json.Marshal(reviewReq)
	req := httptest.NewRequest(http.MethodPut, "/report/review?id="+report.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.ReviewReport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestReportHandler_ReviewReport_NotFound(t *testing.T) {
	handler, _, _, _, _, adminID := setupReportHandlerTest(t)

	reviewReq := ReviewReportRequest{
		Status:      models.ReportStatusResolved,
		ReviewNotes: "Reviewed",
	}
	body, _ := json.Marshal(reviewReq)
	req := httptest.NewRequest(http.MethodPut, "/report/review?id="+uuid.New().String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.ReviewReport(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestReportHandler_DeleteReport(t *testing.T) {
	handler, db, _, reporterID, roomID, adminID := setupReportHandlerTest(t)

	report := models.Report{
		ID:          uuid.New(),
		ReporterID:  reporterID,
		ReportedID:  uuid.New(),
		RoomID:      roomID,
		Reason:      models.ReportReasonSpam,
		Description: "Test report",
		Status:      models.ReportStatusPending,
	}
	db.Create(&report)

	req := httptest.NewRequest(http.MethodDelete, "/report/delete?id="+report.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.DeleteReport(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestReportHandler_DeleteReport_NotFound(t *testing.T) {
	handler, _, _, _, _, adminID := setupReportHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/report/delete?id="+uuid.New().String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.DeleteReport(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestReportHandler_GetMyReports(t *testing.T) {
	handler, db, reporterID, reportedID, roomID, _ := setupReportHandlerTest(t)

	report := models.Report{
		ID:          uuid.New(),
		ReporterID:  reporterID,
		ReportedID:  reportedID,
		RoomID:      roomID,
		Reason:      models.ReportReasonSpam,
		Description: "My test report",
		Status:      models.ReportStatusPending,
	}
	db.Create(&report)

	req := httptest.NewRequest(http.MethodGet, "/reports/my", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), reporterID))
	w := httptest.NewRecorder()

	handler.GetMyReports(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}
