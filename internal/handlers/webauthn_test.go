package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/services"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupWebAuthnHandlerDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.WebAuthnCredential{},
		&models.WebAuthnSession{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupWebAuthnHandlerTest(t *testing.T) (*WebAuthnHandler, *database.Database, uuid.UUID) {
	db := setupWebAuthnHandlerDB(t)
	testDB := &database.Database{DB: db}
	webAuthnService := services.NewWebAuthnService(db, "localhost")
	handler := NewWebAuthnHandler(testDB, webAuthnService)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestWebAuthnHandler_GetStatus_NoCredentials(t *testing.T) {
	handler, _, userID := setupWebAuthnHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/webauthn/status", nil)
	ctx := addUserIDToContext(req.Context(), userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["has_credentials"] != false {
		t.Errorf("expected has_credentials=false, got %v", response["has_credentials"])
	}

	if response["credential_count"].(float64) != 0 {
		t.Errorf("expected credential_count=0, got %v", response["credential_count"])
	}
}

func TestWebAuthnHandler_GetStatus_Unauthorized(t *testing.T) {
	handler, _, _ := setupWebAuthnHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/webauthn/status", nil)
	w := httptest.NewRecorder()

	handler.GetStatus(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestWebAuthnHandler_GetCredentials_Empty(t *testing.T) {
	handler, _, userID := setupWebAuthnHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/webauthn/credentials", nil)
	ctx := addUserIDToContext(req.Context(), userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetCredentials(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response []CredentialResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(response) != 0 {
		t.Errorf("expected 0 credentials, got %d", len(response))
	}
}

func TestWebAuthnHandler_BeginRegistration_Unauthorized(t *testing.T) {
	handler, _, _ := setupWebAuthnHandlerTest(t)

	body, _ := json.Marshal(BeginRegistrationRequest{Name: "My Passkey"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/webauthn/register/begin", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.BeginRegistration(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestWebAuthnHandler_BeginRegistration_EmptyName(t *testing.T) {
	handler, _, userID := setupWebAuthnHandlerTest(t)

	body, _ := json.Marshal(BeginRegistrationRequest{Name: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/webauthn/register/begin", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := addUserIDToContext(req.Context(), userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.BeginRegistration(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestWebAuthnHandler_BeginRegistration_Success(t *testing.T) {
	handler, _, userID := setupWebAuthnHandlerTest(t)

	body, _ := json.Marshal(BeginRegistrationRequest{Name: "My Passkey"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/webauthn/register/begin", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := addUserIDToContext(req.Context(), userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.BeginRegistration(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["challenge"] == nil {
		t.Error("expected challenge in response")
	}

	if response["rp"] == nil {
		t.Error("expected rp in response")
	}

	if response["user"] == nil {
		t.Error("expected user in response")
	}
}

func TestWebAuthnHandler_BeginLogin_NoCredentials(t *testing.T) {
	handler, _, userID := setupWebAuthnHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/webauthn/login/begin", nil)
	ctx := addUserIDToContext(req.Context(), userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.BeginLogin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestWebAuthnHandler_DeleteCredential_NotFound(t *testing.T) {
	handler, _, userID := setupWebAuthnHandlerTest(t)

	body, _ := json.Marshal(map[string]string{"credential_id": "nonexistent"})
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/webauthn/credentials", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := addUserIDToContext(req.Context(), userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.DeleteCredential(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestWebAuthnHandler_DeleteCredential_EmptyID(t *testing.T) {
	handler, _, userID := setupWebAuthnHandlerTest(t)

	body, _ := json.Marshal(map[string]string{"credential_id": ""})
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/webauthn/credentials", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := addUserIDToContext(req.Context(), userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.DeleteCredential(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestWebAuthnHandler_FinishRegistration_MissingFields(t *testing.T) {
	handler, _, userID := setupWebAuthnHandlerTest(t)

	body, _ := json.Marshal(FinishRegistrationRequest{
		Name: "My Passkey",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/webauthn/register/finish", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := addUserIDToContext(req.Context(), userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.FinishRegistration(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestWebAuthnHandler_FinishLogin_MissingFields(t *testing.T) {
	handler, _, userID := setupWebAuthnHandlerTest(t)

	body, _ := json.Marshal(FinishLoginRequest{
		CredentialID: "some-id",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/webauthn/login/finish", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := addUserIDToContext(req.Context(), userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.FinishLogin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestWebAuthnHandler_MethodNotAllowed(t *testing.T) {
	handler, _, userID := setupWebAuthnHandlerTest(t)

	tests := []struct {
		name   string
		method string
		fn     func(http.ResponseWriter, *http.Request)
	}{
		{"GetStatus POST", http.MethodPost, handler.GetStatus},
		{"GetCredentials POST", http.MethodPost, handler.GetCredentials},
		{"BeginRegistration GET", http.MethodGet, handler.BeginRegistration},
		{"BeginLogin GET", http.MethodGet, handler.BeginLogin},
		{"FinishRegistration GET", http.MethodGet, handler.FinishRegistration},
		{"FinishLogin GET", http.MethodGet, handler.FinishLogin},
		{"DeleteCredential GET", http.MethodGet, handler.DeleteCredential},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/auth/webauthn/test", nil)
			ctx := addUserIDToContext(req.Context(), userID)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			tt.fn(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
			}
		})
	}
}
