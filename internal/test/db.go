package test

import (
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var testDB *gorm.DB

func GetTestDB(t *testing.T) *gorm.DB {
	if testDB != nil {
		return testDB
	}

	var db *gorm.DB
	var err error

	if os.Getenv("NEON_DB_HOST") != "" {
		db, err = connectNeon()
	} else {
		db, err = connectSQLite()
	}

	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	testDB = db
	return db
}

func connectNeon() (*gorm.DB, error) {
	host := getEnv("NEON_DB_HOST", "localhost")
	port := getEnv("NEON_DB_PORT", "5432")
	user := getEnv("NEON_DB_USER", "postgres")
	password := getEnv("NEON_DB_PASSWORD", "postgres")
	dbname := getEnv("NEON_DB_NAME", "postgres")
	sslmode := getEnv("NEON_DB_SSLMODE", "require")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func connectSQLite() (*gorm.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.New().String())
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
