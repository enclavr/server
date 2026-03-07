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

	if os.Getenv("POSTGRES_HOST") != "" {
		db, err = connectPostgres()
	} else {
		db, err = connectSQLite()
	}

	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	testDB = db
	return db
}

func connectPostgres() (*gorm.DB, error) {
	host := getEnv("POSTGRES_HOST", "localhost")
	port := getEnv("POSTGRES_PORT", "5432")
	user := getEnv("POSTGRES_USER", "postgres")
	password := getEnv("POSTGRES_PASSWORD", "postgres")
	dbname := getEnv("POSTGRES_DB", "postgres")
	sslmode := getEnv("POSTGRES_SSLMODE", "require")

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
