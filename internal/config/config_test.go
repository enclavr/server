package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	_ = os.Unsetenv("SERVER_PORT")
	_ = os.Unsetenv("DB_HOST")
	_ = os.Setenv("JWT_SECRET", "test-jwt-secret-for-testing")
	_ = os.Unsetenv("ALLOWED_ORIGINS")

	cfg := Load()

	if cfg.Server.Port != "8080" {
		t.Errorf("Expected port 8080, got %s", cfg.Server.Port)
	}

	if cfg.Database.Host != "localhost" {
		t.Errorf("Expected DB host localhost, got %s", cfg.Database.Host)
	}

	if cfg.Auth.JWTSecret == "" {
		t.Error("JWT secret should not be empty")
	}

	if cfg.Auth.JWTExpiration != 24*time.Hour {
		t.Errorf("Expected JWT expiration 24h, got %v", cfg.Auth.JWTExpiration)
	}

	if cfg.Auth.RefreshExpiration != 7*24*time.Hour {
		t.Errorf("Expected refresh expiration 7d, got %v", cfg.Auth.RefreshExpiration)
	}
}

func TestLoadWithEnvVars(t *testing.T) {
	_ = os.Setenv("SERVER_PORT", "9000")
	_ = os.Setenv("DB_HOST", "db.example.com")
	_ = os.Setenv("DB_PORT", "5433")
	_ = os.Setenv("JWT_SECRET", "custom-secret")
	_ = os.Setenv("JWT_EXPIRATION", "2h")
	_ = os.Setenv("REFRESH_EXPIRATION", "336h")
	_ = os.Setenv("OIDC_ENABLED", "true")
	_ = os.Setenv("ALLOWED_ORIGINS", "http://localhost:3000,https://example.com")
	_ = os.Setenv("MAX_UPLOAD_SIZE_MB", "50")

	defer func() {
		_ = os.Unsetenv("SERVER_PORT")
		_ = os.Unsetenv("DB_HOST")
		_ = os.Unsetenv("DB_PORT")
		_ = os.Unsetenv("JWT_SECRET")
		_ = os.Unsetenv("JWT_EXPIRATION")
		_ = os.Unsetenv("REFRESH_EXPIRATION")
		_ = os.Unsetenv("OIDC_ENABLED")
		_ = os.Unsetenv("ALLOWED_ORIGINS")
		_ = os.Unsetenv("MAX_UPLOAD_SIZE_MB")
	}()

	cfg := Load()

	if cfg.Server.Port != "9000" {
		t.Errorf("Expected port 9000, got %s", cfg.Server.Port)
	}

	if cfg.Database.Host != "db.example.com" {
		t.Errorf("Expected DB host db.example.com, got %s", cfg.Database.Host)
	}

	if cfg.Database.Port != "5433" {
		t.Errorf("Expected DB port 5433, got %s", cfg.Database.Port)
	}

	if cfg.Auth.JWTSecret != "custom-secret" {
		t.Errorf("Expected JWT secret custom-secret, got %s", cfg.Auth.JWTSecret)
	}

	if cfg.Auth.JWTExpiration != 2*time.Hour {
		t.Errorf("Expected JWT expiration 2h, got %v", cfg.Auth.JWTExpiration)
	}

	if cfg.Auth.RefreshExpiration != 14*24*time.Hour {
		t.Errorf("Expected refresh expiration 14d (%v), got %v", 14*24*time.Hour, cfg.Auth.RefreshExpiration)
	}

	if !cfg.Auth.OIDCEnabled {
		t.Error("Expected OIDC enabled")
	}

	if len(cfg.Server.AllowedOrigins) != 2 {
		t.Errorf("Expected 2 allowed origins, got %d", len(cfg.Server.AllowedOrigins))
	}

	if cfg.Server.MaxUploadSizeMB != 50 {
		t.Errorf("Expected max upload size 50, got %d", cfg.Server.MaxUploadSizeMB)
	}
}

func TestGetEnv(t *testing.T) {
	_ = os.Setenv("TEST_KEY", "test_value")
	defer func() { _ = os.Unsetenv("TEST_KEY") }()

	if getEnv("TEST_KEY", "default") != "test_value" {
		t.Error("Expected test_value")
	}

	if getEnv("NONEXISTENT_KEY", "default") != "default" {
		t.Error("Expected default")
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		value        string
		defaultValue bool
		expected     bool
	}{
		{"true value", "BOOL_TEST", "true", false, true},
		{"false value", "BOOL_TEST2", "false", true, false},
		{"1 value", "BOOL_TEST3", "1", false, true},
		{"0 value", "BOOL_TEST4", "0", true, false},
		{"invalid value", "BOOL_TEST5", "yes", true, false},
		{"unset key", "NONEXISTENT_BOOL", "", true, true},
		{"unset key false", "NONEXISTENT_BOOL2", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				_ = os.Setenv(tt.key, tt.value)
				defer func() { _ = os.Unsetenv(tt.key) }()
			}
			result := getEnvBool(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	_ = os.Setenv("INT_TEST", "42")
	defer func() { _ = os.Unsetenv("INT_TEST") }()

	if getEnvInt("INT_TEST", 0) != 42 {
		t.Error("Expected 42")
	}

	if getEnvInt("NONEXISTENT_INT", 100) != 100 {
		t.Error("Expected default 100")
	}

	_ = os.Setenv("INVALID_INT", "abc")
	defer func() { _ = os.Unsetenv("INVALID_INT") }()

	if getEnvInt("INVALID_INT", 50) != 50 {
		t.Error("Expected default 50 for invalid int")
	}
}

func TestGetDurationEnv(t *testing.T) {
	_ = os.Setenv("DURATION_TEST", "30s")
	defer func() { _ = os.Unsetenv("DURATION_TEST") }()

	result := getDurationEnv("DURATION_TEST", time.Minute)
	if result != 30*time.Second {
		t.Errorf("Expected 30s, got %v", result)
	}

	result = getDurationEnv("NONEXISTENT_DURATION", time.Hour)
	if result != time.Hour {
		t.Errorf("Expected 1h, got %v", result)
	}

	_ = os.Setenv("INVALID_DURATION", "invalid")
	defer func() { _ = os.Unsetenv("INVALID_DURATION") }()

	result = getDurationEnv("INVALID_DURATION", 2*time.Hour)
	if result != 2*time.Hour {
		t.Errorf("Expected 2h for invalid, got %v", result)
	}
}

func TestDatabaseDSN(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "user",
		Password: "password",
		DBName:   "testdb",
		SSLMode:  "disable",
	}

	dsn := cfg.DSN()
	expected := "host=localhost port=5432 user=user password=password dbname=testdb sslmode=disable"
	if dsn != expected {
		t.Errorf("Expected %s, got %s", expected, dsn)
	}
}

func TestLoadVoiceConfig(t *testing.T) {
	_ = os.Setenv("JWT_SECRET", "test-jwt-secret-for-testing")
	_ = os.Setenv("STUN_SERVER", "stun:custom.stun.com:3478")
	_ = os.Setenv("TURN_SERVER", "turn:custom.turn.com:3478")
	_ = os.Setenv("TURN_USER", "user")
	_ = os.Setenv("TURN_PASS", "pass")

	defer func() {
		_ = os.Unsetenv("STUN_SERVER")
		_ = os.Unsetenv("TURN_SERVER")
		_ = os.Unsetenv("TURN_USER")
		_ = os.Unsetenv("TURN_PASS")
	}()

	cfg := Load()

	if cfg.Voice.STUNServer != "stun:custom.stun.com:3478" {
		t.Errorf("Expected custom STUN server, got %s", cfg.Voice.STUNServer)
	}

	if cfg.Voice.TURNServer != "turn:custom.turn.com:3478" {
		t.Errorf("Expected custom TURN server, got %s", cfg.Voice.TURNServer)
	}

	if cfg.Voice.TURNUser != "user" {
		t.Errorf("Expected TURN user, got %s", cfg.Voice.TURNUser)
	}

	if cfg.Voice.TURNPass != "pass" {
		t.Errorf("Expected TURN pass, got %s", cfg.Voice.TURNPass)
	}
}

func TestLoadRedisConfig(t *testing.T) {
	_ = os.Setenv("JWT_SECRET", "test-jwt-secret-for-testing")
	_ = os.Setenv("REDIS_HOST", "redis.example.com")
	_ = os.Setenv("REDIS_PORT", "6380")
	_ = os.Setenv("REDIS_PASSWORD", "redispass")
	_ = os.Setenv("REDIS_DB", "5")

	defer func() {
		_ = os.Unsetenv("REDIS_HOST")
		_ = os.Unsetenv("REDIS_PORT")
		_ = os.Unsetenv("REDIS_PASSWORD")
		_ = os.Unsetenv("REDIS_DB")
	}()

	cfg := Load()

	if cfg.Redis.Host != "redis.example.com" {
		t.Errorf("Expected redis.example.com, got %s", cfg.Redis.Host)
	}

	if cfg.Redis.Port != "6380" {
		t.Errorf("Expected 6380, got %s", cfg.Redis.Port)
	}

	if cfg.Redis.Password != "redispass" {
		t.Errorf("Expected redispass, got %s", cfg.Redis.Password)
	}

	if cfg.Redis.DB != 5 {
		t.Errorf("Expected DB 5, got %d", cfg.Redis.DB)
	}
}

func TestLoadPushConfig(t *testing.T) {
	_ = os.Setenv("JWT_SECRET", "test-jwt-secret-for-testing")
	_ = os.Setenv("VAPID_PUBLIC_KEY", "publickey")
	_ = os.Setenv("VAPID_PRIVATE_KEY", "privatekey")
	_ = os.Setenv("VAPID_SUBJECT", "mailto:test@example.com")

	defer func() {
		_ = os.Unsetenv("VAPID_PUBLIC_KEY")
		_ = os.Unsetenv("VAPID_PRIVATE_KEY")
		_ = os.Unsetenv("VAPID_SUBJECT")
	}()

	cfg := Load()

	if cfg.Push.VAPIDPublicKey != "publickey" {
		t.Errorf("Expected publickey, got %s", cfg.Push.VAPIDPublicKey)
	}

	if cfg.Push.VAPIDPrivateKey != "privatekey" {
		t.Errorf("Expected privatekey, got %s", cfg.Push.VAPIDPrivateKey)
	}

	if cfg.Push.VAPIDSubject != "mailto:test@example.com" {
		t.Errorf("Expected mailto:test@example.com, got %s", cfg.Push.VAPIDSubject)
	}
}
