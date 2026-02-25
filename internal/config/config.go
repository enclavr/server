package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Voice    VoiceConfig
	Push     PushConfig
}

type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	AllowedOrigins  []string
	UploadDir       string
	MaxUploadSizeMB int
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
	JWTSecret         string
	JWTExpiration     time.Duration
	RefreshExpiration time.Duration
	OIDCEnabled       bool
	OIDCIssuerURL     string
	OIDCClientID      string
	OIDCClientSecret  string
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

func Load() *Config {
	allowedOriginsEnv := getEnv("ALLOWED_ORIGINS", "")
	var allowedOrigins []string
	if allowedOriginsEnv != "" {
		allowedOrigins = strings.Split(allowedOriginsEnv, ",")
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
			JWTSecret:         getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
			JWTExpiration:     getDurationEnv("JWT_EXPIRATION", 24*time.Hour),
			RefreshExpiration: getDurationEnv("REFRESH_EXPIRATION", 7*24*time.Hour),
			OIDCEnabled:       getEnvBool("OIDC_ENABLED", false),
			OIDCIssuerURL:     getEnv("OIDC_ISSUER_URL", ""),
			OIDCClientID:      getEnv("OIDC_CLIENT_ID", ""),
			OIDCClientSecret:  getEnv("OIDC_CLIENT_SECRET", ""),
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
