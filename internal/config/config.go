package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Email    EmailConfig
	Voice    VoiceConfig
	Push     PushConfig
	Redis    RedisConfig
	Sentry   SentryConfig
	Admin    AdminConfig
}

type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	AllowedOrigins  []string
	UploadDir       string
	MaxUploadSizeMB int
}

type AdminConfig struct {
	Username     string
	Password     string
	Email        string
	FirstIsAdmin bool
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type AuthConfig struct {
	JWTSecret           string
	JWTExpiration       time.Duration
	RefreshExpiration   time.Duration
	EncryptionKey       string
	OIDCEnabled         bool
	OIDCIssuerURL       string
	OIDCClientID        string
	OIDCClientSecret    string
	OAuthEnabled        bool
	GoogleClientID      string
	GoogleClientSecret  string
	GitHubClientID      string
	GitHubClientSecret  string
	DiscordClientID     string
	DiscordClientSecret string
}

type EmailConfig struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	UseTLS       bool
}

type VoiceConfig struct {
	STUNServer string
	TURNServer string
	TURNUser   string
	TURNPass   string
}

type PushConfig struct {
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type SentryConfig struct {
	DSN         string
	Environment string
}

func Load() *Config {
	allowedOriginsEnv := getEnv("ALLOWED_ORIGINS", "")
	var allowedOrigins []string
	if allowedOriginsEnv != "" {
		allowedOrigins = strings.Split(allowedOriginsEnv, ",")
	}

	jwtSecret := getEnv("JWT_SECRET", "")
	if jwtSecret == "" || jwtSecret == "your-secret-key-change-in-production" {
		log.Fatal("FATAL: JWT_SECRET must be set and not be the default value. Set JWT_SECRET environment variable.")
	}

	return &Config{
		Server: ServerConfig{
			Port:            getEnv("SERVER_PORT", "8080"),
			ReadTimeout:     getDurationEnv("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getDurationEnv("SERVER_WRITE_TIMEOUT", 30*time.Second),
			AllowedOrigins:  allowedOrigins,
			UploadDir:       getEnv("UPLOAD_DIR", "./uploads"),
			MaxUploadSizeMB: getEnvInt("MAX_UPLOAD_SIZE_MB", 10),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "enclavr"),
			Password: getEnv("DB_PASSWORD", "enclavr"),
			DBName:   getEnv("DB_NAME", "enclavr"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Auth: AuthConfig{
			JWTSecret:           getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
			JWTExpiration:       getDurationEnv("JWT_EXPIRATION", 24*time.Hour),
			RefreshExpiration:   getDurationEnv("REFRESH_EXPIRATION", 7*24*time.Hour),
			EncryptionKey:       getEnv("ENCRYPTION_KEY", ""),
			OIDCEnabled:         getEnvBool("OIDC_ENABLED", false),
			OIDCIssuerURL:       getEnv("OIDC_ISSUER_URL", ""),
			OIDCClientID:        getEnv("OIDC_CLIENT_ID", ""),
			OIDCClientSecret:    getEnv("OIDC_CLIENT_SECRET", ""),
			OAuthEnabled:        getEnvBool("OAUTH_ENABLED", false),
			GoogleClientID:      getEnv("GOOGLE_CLIENT_ID", ""),
			GoogleClientSecret:  getEnv("GOOGLE_CLIENT_SECRET", ""),
			GitHubClientID:      getEnv("GITHUB_CLIENT_ID", ""),
			GitHubClientSecret:  getEnv("GITHUB_CLIENT_SECRET", ""),
			DiscordClientID:     getEnv("DISCORD_CLIENT_ID", ""),
			DiscordClientSecret: getEnv("DISCORD_CLIENT_SECRET", ""),
		},
		Email: EmailConfig{
			SMTPHost:     getEnv("SMTP_HOST", ""),
			SMTPPort:     getEnv("SMTP_PORT", "587"),
			SMTPUsername: getEnv("SMTP_USERNAME", ""),
			SMTPPassword: getEnv("SMTP_PASSWORD", ""),
			SMTPFrom:     getEnv("SMTP_FROM", "noreply@enclavr.local"),
			UseTLS:       getEnvBool("SMTP_USE_TLS", true),
		},
		Voice: VoiceConfig{
			STUNServer: getEnv("STUN_SERVER", "stun:stun.l.google.com:19302"),
			TURNServer: getEnv("TURN_SERVER", ""),
			TURNUser:   getEnv("TURN_USER", ""),
			TURNPass:   getEnv("TURN_PASS", ""),
		},
		Push: PushConfig{
			VAPIDPublicKey:  getEnv("VAPID_PUBLIC_KEY", ""),
			VAPIDPrivateKey: getEnv("VAPID_PRIVATE_KEY", ""),
			VAPIDSubject:    getEnv("VAPID_SUBJECT", "mailto:admin@enclavr.local"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Sentry: SentryConfig{
			DSN:         getEnv("SENTRY_DSN", ""),
			Environment: getEnv("SENTRY_ENVIRONMENT", "development"),
		},
		Admin: AdminConfig{
			Username:     getEnv("ADMIN_USERNAME", "admin"),
			Password:     getEnv("ADMIN_PASSWORD", ""),
			Email:        getEnv("ADMIN_EMAIL", "admin@enclavr.local"),
			FirstIsAdmin: getEnvBool("FIRST_USER_IS_ADMIN", true),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intVal int
		if _, err := fmt.Sscanf(value, "%d", &intVal); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func (d *DatabaseConfig) DSN() string {
	return "host=" + d.Host +
		" port=" + d.Port +
		" user=" + d.User +
		" password=" + d.Password +
		" dbname=" + d.DBName +
		" sslmode=" + d.SSLMode
}
