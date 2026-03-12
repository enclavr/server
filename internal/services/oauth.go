package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"
)

type OAuthProvider string

const (
	ProviderGoogle OAuthProvider = "google"
	ProviderGitHub OAuthProvider = "github"
)

type OAuthUserInfo struct {
	Provider  OAuthProvider
	Subject   string
	Email     string
	Name      string
	AvatarURL string
}

type OAuthService struct {
	googleConfig *oauth2.Config
	githubConfig *oauth2.Config
	enabled      bool
}

func NewOAuthService(cfg *config.AuthConfig) *OAuthService {
	s := &OAuthService{
		enabled: cfg.OAuthEnabled,
	}

	if cfg.OAuthEnabled {
		if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
			s.googleConfig = &oauth2.Config{
				ClientID:     cfg.GoogleClientID,
				ClientSecret: cfg.GoogleClientSecret,
				Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
				Endpoint:     google.Endpoint,
			}
		}

		if cfg.GitHubClientID != "" && cfg.GitHubClientSecret != "" {
			s.githubConfig = &oauth2.Config{
				ClientID:     cfg.GitHubClientID,
				ClientSecret: cfg.GitHubClientSecret,
				Scopes:       []string{"user:email", "read:user"},
				Endpoint:     github.Endpoint,
			}
		}
	}

	return s
}

func (s *OAuthService) IsEnabled() bool {
	return s.enabled
}

func (s *OAuthService) IsProviderEnabled(provider OAuthProvider) bool {
	if !s.enabled {
		return false
	}
	switch provider {
	case ProviderGoogle:
		return s.googleConfig != nil
	case ProviderGitHub:
		return s.githubConfig != nil
	}
	return false
}

func (s *OAuthService) GetAuthURL(provider OAuthProvider, state string, redirectURI string) (string, error) {
	var config *oauth2.Config
	switch provider {
	case ProviderGoogle:
		config = s.googleConfig
	case ProviderGitHub:
		config = s.githubConfig
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}

	if config == nil {
		return "", fmt.Errorf("provider %s not configured", provider)
	}

	return config.AuthCodeURL(state, oauth2.AccessTypeOnline), nil
}

func (s *OAuthService) ExchangeCode(ctx context.Context, provider OAuthProvider, code string, redirectURI string) (*oauth2.Token, error) {
	var config *oauth2.Config
	switch provider {
	case ProviderGoogle:
		config = s.googleConfig
	case ProviderGitHub:
		config = s.githubConfig
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	if config == nil {
		return nil, fmt.Errorf("provider %s not configured", provider)
	}

	token, err := config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("code exchange failed: %w", err)
	}

	return token, nil
}

func (s *OAuthService) GetUserInfo(ctx context.Context, provider OAuthProvider, token *oauth2.Token) (*OAuthUserInfo, error) {
	switch provider {
	case ProviderGoogle:
		return s.getGoogleUserInfo(ctx, token)
	case ProviderGitHub:
		return s.getGitHubUserInfo(ctx, token)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func (s *OAuthService) getGoogleUserInfo(ctx context.Context, token *oauth2.Token) (*OAuthUserInfo, error) {
	client := s.googleConfig.Client(ctx, token)

	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google userinfo returned status %d", resp.StatusCode)
	}

	var data struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		VerifiedEmail bool   `json:"verified_email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	if data.Email == "" || !data.VerifiedEmail {
		return nil, fmt.Errorf("email not verified")
	}

	return &OAuthUserInfo{
		Provider:  ProviderGoogle,
		Subject:   data.ID,
		Email:     data.Email,
		Name:      data.Name,
		AvatarURL: data.Picture,
	}, nil
}

func (s *OAuthService) getGitHubUserInfo(ctx context.Context, token *oauth2.Token) (*OAuthUserInfo, error) {
	client := s.githubConfig.Client(ctx, token)

	userResp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	defer userResp.Body.Close()

	if userResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github user returned status %d", userResp.StatusCode)
	}

	var userData struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Email     string `json:"email"`
	}

	if err := json.NewDecoder(userResp.Body).Decode(&userData); err != nil {
		return nil, fmt.Errorf("failed to decode user data: %w", err)
	}

	email := userData.Email
	if email == "" {
		email, err = s.getGitHubPrimaryEmail(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("failed to get primary email: %w", err)
		}
	}

	if email == "" {
		return nil, fmt.Errorf("no email found")
	}

	name := userData.Name
	if name == "" {
		name = userData.Login
	}

	return &OAuthUserInfo{
		Provider:  ProviderGitHub,
		Subject:   fmt.Sprintf("%d", userData.ID),
		Email:     email,
		Name:      name,
		AvatarURL: userData.AvatarURL,
	}, nil
}

func (s *OAuthService) getGitHubPrimaryEmail(ctx context.Context, token *oauth2.Token) (string, error) {
	client := s.githubConfig.Client(ctx, token)

	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github emails returned status %d", resp.StatusCode)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}

	return "", nil
}

func (s *OAuthService) FindOrCreateUser(db *gorm.DB, userInfo *OAuthUserInfo) (*models.User, error) {
	var user models.User

	result := db.Where("email = ?", userInfo.Email).First(&user)
	if result.Error == nil {
		return &user, nil
	}
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}

	username := generateOAuthUsername(userInfo.Name, userInfo.Email)

	newUser := models.User{
		ID:          uuid.New(),
		Username:    username,
		Email:       userInfo.Email,
		DisplayName: userInfo.Name,
		AvatarURL:   userInfo.AvatarURL,
		OIDCIssuer:  string(userInfo.Provider),
		OIDCSubject: userInfo.Subject,
	}

	if err := db.Create(&newUser).Error; err != nil {
		return nil, err
	}

	return &newUser, nil
}

func generateOAuthUsername(name, email string) string {
	emailPart := strings.Split(email, "@")[0]
	emailPart = strings.ReplaceAll(emailPart, ".", "_")
	emailPart = strings.ReplaceAll(emailPart, "+", "_")

	if name != "" {
		nameLower := strings.ToLower(strings.ReplaceAll(name, " ", "_"))
		return nameLower
	}

	return emailPart
}

var _ = io.Discard
