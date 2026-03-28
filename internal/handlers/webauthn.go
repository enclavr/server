package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/services"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

// WebAuthnHandler handles WebAuthn/passkey registration and authentication endpoints.
type WebAuthnHandler struct {
	db              *database.Database
	webAuthnService *services.WebAuthnService
}

// NewWebAuthnHandler creates a new WebAuthnHandler.
func NewWebAuthnHandler(db *database.Database, webAuthnService *services.WebAuthnService) *WebAuthnHandler {
	return &WebAuthnHandler{
		db:              db,
		webAuthnService: webAuthnService,
	}
}

// BeginRegistrationRequest is the request body for starting passkey registration.
type BeginRegistrationRequest struct {
	Name string `json:"name"`
}

// FinishRegistrationRequest is the request body for completing passkey registration.
type FinishRegistrationRequest struct {
	Name              string `json:"name"`
	CredentialID      string `json:"credential_id"`
	AttestationObject string `json:"attestation_object"`
	ClientDataJSON    string `json:"client_data_json"`
}

// FinishLoginRequest is the request body for completing passkey authentication.
type FinishLoginRequest struct {
	CredentialID      string `json:"credential_id"`
	ClientDataJSON    string `json:"client_data_json"`
	AuthenticatorData string `json:"authenticator_data"`
	Signature         string `json:"signature"`
}

// CredentialResponse is the response for credential listing.
type CredentialResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CredentialID string `json:"credential_id"`
	CreatedAt    string `json:"created_at"`
	SignCount    uint32 `json:"sign_count"`
}

// BeginRegistration starts the WebAuthn registration ceremony.
// POST /api/auth/webauthn/register/begin
func (h *WebAuthnHandler) BeginRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req BeginRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Credential name is required", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	options, _, err := h.webAuthnService.BeginRegistration(r.Context(), userID, user.Username)
	if err != nil {
		log.Printf("WebAuthn begin registration error: %v", err)
		http.Error(w, "Failed to begin registration", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(options); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// FinishRegistration completes the WebAuthn registration ceremony.
// POST /api/auth/webauthn/register/finish
func (h *WebAuthnHandler) FinishRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req FinishRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.CredentialID == "" || req.AttestationObject == "" || req.ClientDataJSON == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	credentialData := map[string]interface{}{
		"id":                req.CredentialID,
		"attestationObject": req.AttestationObject,
		"clientDataJSON":    req.ClientDataJSON,
	}

	credential, err := h.webAuthnService.FinishRegistration(r.Context(), userID, req.Name, credentialData)
	if err != nil {
		log.Printf("WebAuthn finish registration error: %v", err)
		http.Error(w, "Failed to complete registration: "+err.Error(), http.StatusBadRequest)
		return
	}

	response := CredentialResponse{
		ID:           credential.ID.String(),
		Name:         credential.Name,
		CredentialID: credential.CredentialID,
		CreatedAt:    credential.CreatedAt.Format("2006-01-02T15:04:05Z"),
		SignCount:    credential.SignCount,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

// BeginLogin starts the WebAuthn authentication ceremony.
// POST /api/auth/webauthn/login/begin
func (h *WebAuthnHandler) BeginLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	options, _, err := h.webAuthnService.BeginLogin(r.Context(), userID)
	if err != nil {
		log.Printf("WebAuthn begin login error: %v", err)
		http.Error(w, "Failed to begin login: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(options); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// FinishLogin completes the WebAuthn authentication ceremony.
// POST /api/auth/webauthn/login/finish
func (h *WebAuthnHandler) FinishLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req FinishLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.CredentialID == "" || req.ClientDataJSON == "" || req.AuthenticatorData == "" || req.Signature == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	assertionData := map[string]interface{}{
		"credentialId":      req.CredentialID,
		"clientDataJSON":    req.ClientDataJSON,
		"authenticatorData": req.AuthenticatorData,
		"signature":         req.Signature,
	}

	credential, err := h.webAuthnService.FinishLogin(r.Context(), userID, req.CredentialID, assertionData)
	if err != nil {
		log.Printf("WebAuthn finish login error: %v", err)
		http.Error(w, "Authentication failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	response := CredentialResponse{
		ID:           credential.ID.String(),
		Name:         credential.Name,
		CredentialID: credential.CredentialID,
		CreatedAt:    credential.CreatedAt.Format("2006-01-02T15:04:05Z"),
		SignCount:    credential.SignCount,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

// GetCredentials returns all registered passkeys for the authenticated user.
// GET /api/auth/webauthn/credentials
func (h *WebAuthnHandler) GetCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	credentials, err := h.webAuthnService.GetCredentials(userID)
	if err != nil {
		log.Printf("WebAuthn get credentials error: %v", err)
		http.Error(w, "Failed to get credentials", http.StatusInternalServerError)
		return
	}

	response := make([]CredentialResponse, 0, len(credentials))
	for _, cred := range credentials {
		response = append(response, CredentialResponse{
			ID:           cred.ID.String(),
			Name:         cred.Name,
			CredentialID: cred.CredentialID,
			CreatedAt:    cred.CreatedAt.Format("2006-01-02T15:04:05Z"),
			SignCount:    cred.SignCount,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

// DeleteCredential removes a registered passkey.
// DELETE /api/auth/webauthn/credentials
func (h *WebAuthnHandler) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		CredentialID string `json:"credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.CredentialID == "" {
		http.Error(w, "credential_id is required", http.StatusBadRequest)
		return
	}

	var credential models.WebAuthnCredential
	if err := h.db.Where("credential_id = ? AND user_id = ?", req.CredentialID, userID).First(&credential).Error; err != nil {
		http.Error(w, "Credential not found", http.StatusNotFound)
		return
	}

	if err := h.webAuthnService.DeleteCredential(req.CredentialID); err != nil {
		log.Printf("WebAuthn delete credential error: %v", err)
		http.Error(w, "Failed to delete credential", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

// GetStatus returns whether the user has any registered passkeys.
// GET /api/auth/webauthn/status
func (h *WebAuthnHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	credentials, err := h.webAuthnService.GetCredentials(userID)
	if err != nil {
		log.Printf("WebAuthn get status error: %v", err)
		http.Error(w, "Failed to get status", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"enabled":          h.webAuthnService.IsEnabled(),
		"credential_count": len(credentials),
		"has_credentials":  len(credentials) > 0,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
