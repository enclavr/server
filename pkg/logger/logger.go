package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Level string

const (
	DebugLevel Level = "DEBUG"
	InfoLevel  Level = "INFO"
	WarnLevel  Level = "WARN"
	ErrorLevel Level = "ERROR"
)

var (
	currentLevel = InfoLevel
	levelLock    sync.RWMutex
)

type Logger struct {
	logger *log.Logger
	mu     sync.Mutex
}

type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     Level                  `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
}

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	UserIDKey    contextKey = "user_id"
)

var (
	defaultLogger *Logger
	once          sync.Once
	output        io.Writer = os.Stdout
)

func Init() {
	once.Do(func() {
		defaultLogger = &Logger{
			logger: log.New(output, "", 0),
		}
	})
}

func SetLevel(level Level) {
	levelLock.Lock()
	defer levelLock.Unlock()
	currentLevel = level
}

func GetLevel() Level {
	levelLock.RLock()
	defer levelLock.RUnlock()
	return currentLevel
}

func shouldLog(level Level) bool {
	levelLock.RLock()
	defer levelLock.RUnlock()

	levels := map[Level]int{
		DebugLevel: 0,
		InfoLevel:  1,
		WarnLevel:  2,
		ErrorLevel: 3,
	}

	current := levels[currentLevel]
	msgLevel := levels[level]

	return msgLevel >= current
}

func SetOutput(w io.Writer) {
	output = w
	if defaultLogger != nil {
		defaultLogger.logger.SetOutput(w)
	}
}

func logEntry(level Level, msg string, fields map[string]interface{}) {
	if !shouldLog(level) {
		return
	}

	if defaultLogger == nil {
		Init()
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Message:   msg,
		Fields:    fields,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		defaultLogger.logger.Println("Failed to marshal log entry:", err)
		return
	}

	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.logger.Println(string(data))
}

func logEntryWithContext(ctx context.Context, level Level, msg string, fields map[string]interface{}) {
	if !shouldLog(level) {
		return
	}

	if defaultLogger == nil {
		Init()
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Message:   msg,
		Fields:    fields,
	}

	if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
		entry.RequestID = reqID
	}

	if userID, ok := ctx.Value(UserIDKey).(uuid.UUID); ok && userID != uuid.Nil {
		entry.UserID = userID.String()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		defaultLogger.logger.Println("Failed to marshal log entry:", err)
		return
	}

	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.logger.Println(string(data))
}

func WithContext(ctx context.Context) ContextLogger {
	return ContextLogger{ctx: ctx}
}

type ContextLogger struct {
	ctx context.Context
}

func (c ContextLogger) Debug(msg string, fields map[string]interface{}) {
	logEntryWithContext(c.ctx, DebugLevel, msg, fields)
}

func (c ContextLogger) Info(msg string, fields map[string]interface{}) {
	logEntryWithContext(c.ctx, InfoLevel, msg, fields)
}

func (c ContextLogger) Warn(msg string, fields map[string]interface{}) {
	logEntryWithContext(c.ctx, WarnLevel, msg, fields)
}

func (c ContextLogger) Error(msg string, fields map[string]interface{}) {
	logEntryWithContext(c.ctx, ErrorLevel, msg, fields)
}

func Debug(msg string, fields map[string]interface{}) {
	logEntry(DebugLevel, msg, fields)
}

func Info(msg string, fields map[string]interface{}) {
	logEntry(InfoLevel, msg, fields)
}

func Warn(msg string, fields map[string]interface{}) {
	logEntry(WarnLevel, msg, fields)
}

func Error(msg string, fields map[string]interface{}) {
	logEntry(ErrorLevel, msg, fields)
}

func Fatal(msg string, fields map[string]interface{}) {
	logEntry(ErrorLevel, msg, fields)
	os.Exit(1)
}

func FromContext(ctx context.Context, msg string, fields map[string]interface{}) {
	logEntryWithContext(ctx, InfoLevel, msg, fields)
}

type RequestLogger struct {
	logger *log.Logger
	mu     sync.Mutex
}

var requestLogger *RequestLogger

func InitRequestLogger() {
	requestLogger = &RequestLogger{
		logger: log.New(output, "", 0),
	}
}

func RequestLog(ctx context.Context, method, path, ip string, status int, duration time.Duration, userID *uuid.UUID) {
	if requestLogger == nil {
		InitRequestLogger()
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     InfoLevel,
		Message:   fmt.Sprintf("%s %s - %d - %v", method, path, status, duration),
		Fields: map[string]interface{}{
			"method":   method,
			"path":     path,
			"ip":       ip,
			"status":   status,
			"duration": duration.Milliseconds(),
		},
	}

	if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
		entry.RequestID = reqID
	}

	if userID != nil && *userID != uuid.Nil {
		entry.UserID = userID.String()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Failed to marshal request log entry: %v", err)
		return
	}

	requestLogger.mu.Lock()
	defer requestLogger.mu.Unlock()
	requestLogger.logger.Println(string(data))
}
