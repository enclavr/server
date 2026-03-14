package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

type OAuthHandler struct {
	db          *database.Database
	authService *auth.AuthService
	cfg         *config.AuthConfig
	googleCfg   oauth2.Config
	githubCfg   oauth2.Config
}

type OAuthProvider string

const (
	ProviderGoogle OAuthProvider = "google"
	ProviderGitHub OAuthProvider = "github"
)

func NewOAuthHandler(db *database.Database, authService *auth.AuthService, cfg *config.AuthConfig) *OAuthHandler {
	h := &OAuthHandler{
		db:          db,
		authService: authService,
		cfg:         cfg,
	}

	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		h.googleCfg = oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  "/api/auth/oauth/google/callback",
			Endpoint:     google.Endpoint,
			Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		}
	}

	if cfg.GitHubClientID != "" && cfg.GitHubClientSecret != "" {
		h.githubCfg = oauth2.Config{
			ClientID:     cfg.GitHubClientID,
			ClientSecret: cfg.GitHubClientSecret,
			RedirectURL:  "/api/auth/oauth/github/callback",
			Endpoint:     github.Endpoint,
			Scopes:       []string{"user:email", "read:user"},
		}
	}

	return h
}

func (h *OAuthHandler) IsEnabled() bool {
	return h.cfg.OAuthEnabled
}

func (h *OAuthHandler) GetProviders(w http.ResponseWriter, r *http.Request) {
	providers := map[string]bool{
		"google": h.googleCfg.ClientID != "",
		"github": h.githubCfg.ClientID != "",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providers)
}

func (h *OAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	providerStr := r.URL.Query().Get("provider")
	var provider OAuthProvider
	switch providerStr {
	case "google":
		provider = ProviderGoogle
	case "github":
		provider = ProviderGitHub
	default:
		http.Error(w, "Invalid provider", http.StatusBadRequest)
		return
	}

	var oauthCfg oauth2.Config
	switch provider {
	case ProviderGoogle:
		if h.googleCfg.ClientID == "" {
			http.Error(w, "Google OAuth not configured", http.StatusNotFound)
			return
		}
		oauthCfg = h.googleCfg
	case ProviderGitHub:
		if h.githubCfg.ClientID == "" {
			http.Error(w, "GitHub OAuth not configured", http.StatusNotFound)
			return
		}
		oauthCfg = h.githubCfg
	}

	state, err := auth.GenerateState()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_provider",
		Value:    string(provider),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	url := oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	providerCookie, err := r.Cookie("oauth_provider")
	if err != nil {
		http.Error(w, "Provider cookie not found", http.StatusBadRequest)
		return
	}
	provider := OAuthProvider(providerCookie.Value)

	stateCookie, err := r.Cookie("oauth_state")
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
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_provider",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	var oauthCfg oauth2.Config
	switch provider {
	case ProviderGoogle:
		oauthCfg = h.googleCfg
	case ProviderGitHub:
		oauthCfg = h.githubCfg
	default:
		http.Error(w, "Invalid provider", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	token, err := oauthCfg.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var user *models.User

	switch provider {
	case ProviderGoogle:
		user, err = h.handleGoogleUser(r.Context(), token)
	case ProviderGitHub:
		user, err = h.handleGitHubUser(r.Context(), token)
	}

	if err != nil {
		http.Error(w, "Failed to get user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.sendAuthResponse(w, user, r)
}

func (h *OAuthHandler) handleGoogleUser(ctx context.Context, token *oauth2.Token) (*models.User, error) {
	userInfo, err := h.getGoogleUserInfo(ctx, token)
	if err != nil {
		return nil, err
	}

	return h.findOrCreateOAuthUser(userInfo.Email, userInfo.Name, userInfo.Picture, "google", userInfo.Sub)
}

func (h *OAuthHandler) getGoogleUserInfo(ctx context.Context, token *oauth2.Token) (*googleUserInfo, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var userInfo googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

type googleUserInfo struct {
	Sub           string `json:"id"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Locale        string `json:"locale"`
}

func (h *OAuthHandler) handleGitHubUser(ctx context.Context, token *oauth2.Token) (*models.User, error) {
	userInfo, err := h.getGitHubUserInfo(ctx, token)
	if err != nil {
		return nil, err
	}

	email := userInfo.Email
	if email == "" {
		emails, err := h.getGitHubEmails(ctx, token)
		if err != nil || len(emails) == 0 {
			return nil, fmt.Errorf("no email found for GitHub user")
		}
		for _, e := range emails {
			if e.Primary {
				email = e.Email
				break
			}
		}
		if email == "" {
			email = emails[0].Email
		}
	}

	return h.findOrCreateOAuthUser(email, userInfo.Name, userInfo.AvatarURL, "github", fmt.Sprintf("%d", userInfo.ID))
}

func (h *OAuthHandler) getGitHubUserInfo(ctx context.Context, token *oauth2.Token) (*githubUserInfo, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var userInfo githubUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

func (h *OAuthHandler) getGitHubEmails(ctx context.Context, token *oauth2.Token) ([]githubEmail, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return nil, err
	}

	return emails, nil
}

type githubUserInfo struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func (h *OAuthHandler) findOrCreateOAuthUser(email, name, avatarURL, provider, providerID string) (*models.User, error) {
	var user models.User

	result := h.db.Where("email = ?", email).First(&user)

	if result.Error == nil {
		return &user, nil
	}

	username := strings.ReplaceAll(name, " ", "_")
	if username == "" {
		username = "user_" + uuid.New().String()[:8]
	}

	var count int64
	h.db.Model(&models.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		username = username + "_" + uuid.New().String()[:4]
	}

	user = models.User{
		ID:           uuid.New(),
		Username:     username,
		Email:        email,
		DisplayName:  name,
		AvatarURL:    avatarURL,
		IsAdmin:      false,
		OIDCSubject:  providerID,
		OIDCIssuer:   provider,
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

func (h *OAuthHandler) sendAuthResponse(w http.ResponseWriter, user *models.User, r *http.Request) {
	sessionID := uuid.New()
	accessToken, err := h.authService.GenerateToken(user, sessionID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	refreshToken, err := h.authService.GenerateRefreshToken(user)
	if err != nil {
		http.Error(w, "Failed to generate refresh token", http.StatusInternalServerError)
		return
	}

	session := models.Session{
		ID:        sessionID,
		UserID:    user.ID,
		Token:     accessToken,
		ExpiresAt: time.Now().Add(h.cfg.JWTExpiration),
		CreatedAt: time.Now(),
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	h.db.Create(&session)

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
