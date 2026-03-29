package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type OIDCHandler struct {
	db        *database.Database
	cfg       *config.AuthConfig
	verifier  *oidc.IDTokenVerifier
	oauth2Cfg oauth2.Config
}

func NewOIDCHandler(db *database.Database, cfg *config.AuthConfig) *OIDCHandler {
	if !cfg.OIDCEnabled {
		return &OIDCHandler{
			db:  db,
			cfg: cfg,
		}
	}

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
	if err != nil {
		return &OIDCHandler{
			db:  db,
			cfg: cfg,
		}
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.OIDCClientID,
	})

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		RedirectURL:  "/api/auth/oidc/callback",
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	return &OIDCHandler{
		db:        db,
		cfg:       cfg,
		verifier:  verifier,
		oauth2Cfg: oauth2Cfg,
	}
}

func (h *OIDCHandler) IsEnabled() bool {
	return h.cfg.OIDCEnabled && h.verifier != nil
}

func (h *OIDCHandler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.IsEnabled() {
		http.Error(w, "OIDC not enabled", http.StatusNotFound)
		return
	}

	state, err := auth.GenerateState()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	url := h.oauth2Cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if !h.IsEnabled() {
		http.Error(w, "OIDC not enabled", http.StatusNotFound)
		return
	}

	stateCookie, err := r.Cookie("oidc_state")
	if err != nil {
		http.Error(w, "State cookie not found", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	if state != stateCookie.Value {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
	})

	code := r.URL.Query().Get("code")
	token, err := h.oauth2Cfg.Exchange(r.Context(), code)
	if err != nil {
		log.Printf("[ERROR] OIDC token exchange failed: %v", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	idToken, err := h.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		log.Printf("[ERROR] OIDC ID token verification failed: %v", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	var claims struct {
		Sub               string `json:"sub"`
		Name              string `json:"name"`
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		log.Printf("[ERROR] OIDC claims parsing failed: %v", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	user, err := h.findOrCreateUser(claims)
	if err != nil {
		log.Printf("[ERROR] OIDC user creation failed: %v", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	authService := auth.NewAuthService(h.cfg)
	accessToken, err := authService.GenerateToken(user)
	if err != nil {
		log.Printf("[ERROR] OIDC token generation failed: %v", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	refreshToken, err := authService.GenerateRefreshToken(user)
	if err != nil {
		log.Printf("[ERROR] OIDC refresh token generation failed: %v", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(h.cfg.JWTExpiration),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(h.cfg.RefreshExpiration),
	})

	response := map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    int(h.cfg.JWTExpiration.Seconds()),
		"user": map[string]interface{}{
			"id":           user.ID.String(),
			"username":     user.Username,
			"email":        user.Email,
			"display_name": user.DisplayName,
			"avatar_url":   user.AvatarURL,
			"is_admin":     user.IsAdmin,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *OIDCHandler) findOrCreateUser(claims struct {
	Sub               string `json:"sub"`
	Name              string `json:"name"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
}) (*models.User, error) {
	var user models.User
	result := h.db.Where("oidc_subject = ?", claims.Sub).First(&user)

	if result.Error == nil {
		return &user, nil
	}

	username := claims.PreferredUsername
	if username == "" {
		username = claims.Name
	}
	if username == "" && claims.Email != "" {
		username = strings.Split(claims.Email, "@")[0]
	}
	if username == "" {
		username = "user_" + uuid.New().String()[:8]
	}

	user = models.User{
		ID:           uuid.New(),
		Username:     username,
		Email:        claims.Email,
		DisplayName:  claims.Name,
		AvatarURL:    "",
		IsAdmin:      false,
		OIDCSubject:  claims.Sub,
		OIDCIssuer:   h.cfg.OIDCIssuerURL,
		PasswordHash: "",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	result = h.db.Create(&user)
	if result.Error != nil {
		return nil, result.Error
	}

	return &user, nil
}

func (h *OIDCHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	response := map[string]bool{
		"enabled": h.IsEnabled(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
