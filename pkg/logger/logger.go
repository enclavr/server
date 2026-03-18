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
	Timestamp     string                 `json:"timestamp"`
	Level         Level                  `json:"level"`
	Message       string                 `json:"message"`
	Fields        map[string]interface{} `json:"fields,omitempty"`
	RequestID     string                 `json:"request_id,omitempty"`
	UserID        string                 `json:"user_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	SpanID        string                 `json:"span_id,omitempty"`
}

type contextKey string

const (
	RequestIDKey     contextKey = "request_id"
	UserIDKey        contextKey = "user_id"
	CorrelationIDKey contextKey = "correlation_id"
	SpanIDKey        contextKey = "span_id"
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

	if corrID, ok := ctx.Value(CorrelationIDKey).(string); ok && corrID != "" {
		entry.CorrelationID = corrID
	}

	if spanID, ok := ctx.Value(SpanIDKey).(string); ok && spanID != "" {
		entry.SpanID = spanID
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

func (c ContextLogger) WithField(key string, value interface{}) ContextLogger {
	return c
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

type RequestLogData struct {
	Method        string `json:"method"`
	Path          string `json:"path"`
	Query         string `json:"query,omitempty"`
	IP            string `json:"ip"`
	Status        int    `json:"status"`
	Duration      int64  `json:"duration_ms"`
	RequestSize   int64  `json:"request_size,omitempty"`
	ResponseSize  int64  `json:"response_size,omitempty"`
	UserAgent     string `json:"user_agent,omitempty"`
	Referer       string `json:"referer,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
	UserID        string `json:"user_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

func RequestLog(ctx context.Context, method, path, ip string, status int, duration time.Duration, userID *uuid.UUID) {
	if requestLogger == nil {
		InitRequestLogger()
	}

	level := InfoLevel
	if status >= 500 {
		level = ErrorLevel
	} else if status >= 400 {
		level = WarnLevel
	}

	if !shouldLog(level) {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
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

	if corrID, ok := ctx.Value(CorrelationIDKey).(string); ok && corrID != "" {
		entry.CorrelationID = corrID
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

func LogRequestData(ctx context.Context, data RequestLogData) {
	if requestLogger == nil {
		InitRequestLogger()
	}

	level := InfoLevel
	if data.Status >= 500 {
		level = ErrorLevel
	} else if data.Status >= 400 {
		level = WarnLevel
	}

	if !shouldLog(level) {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Message:   fmt.Sprintf("%s %s - %d - %dms", data.Method, data.Path, data.Status, data.Duration),
		Fields: map[string]interface{}{
			"method":        data.Method,
			"path":          data.Path,
			"query":         data.Query,
			"ip":            data.IP,
			"status":        data.Status,
			"duration":      data.Duration,
			"request_size":  data.RequestSize,
			"response_size": data.ResponseSize,
			"user_agent":    data.UserAgent,
			"referer":       data.Referer,
		},
	}

	if data.RequestID != "" {
		entry.RequestID = data.RequestID
	}

	if data.UserID != "" {
		entry.UserID = data.UserID
	}

	if data.CorrelationID != "" {
		entry.CorrelationID = data.CorrelationID
	}

	dataBytes, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Failed to marshal request log entry: %v", err)
		return
	}

	requestLogger.mu.Lock()
	defer requestLogger.mu.Unlock()
	requestLogger.logger.Println(string(dataBytes))
}

func LogError(ctx context.Context, err error, msg string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["error"] = err.Error()
	fields["error_type"] = fmt.Sprintf("%T", err)
	logEntryWithContext(ctx, ErrorLevel, msg, fields)
}

func LogPanic(ctx context.Context, recovered interface{}, stack []byte) {
	fields := map[string]interface{}{
		"stack": string(stack),
	}

	var errMsg string
	switch e := recovered.(type) {
	case error:
		errMsg = e.Error()
		fields["error_type"] = fmt.Sprintf("%T", e)
	case string:
		errMsg = e
	default:
		errMsg = fmt.Sprintf("%v", recovered)
		fields["error_type"] = fmt.Sprintf("%T", recovered)
	}

	fields["panic"] = errMsg
	logEntryWithContext(ctx, ErrorLevel, "PANIC RECOVERED", fields)
}

type PerformanceMetrics struct {
	Duration  time.Duration
	Operation string
	Component string
	Metadata  map[string]interface{}
}

func LogPerformance(ctx context.Context, metrics PerformanceMetrics) {
	fields := map[string]interface{}{
		"operation":   metrics.Operation,
		"component":   metrics.Component,
		"duration_ms": metrics.Duration.Milliseconds(),
	}

	if metrics.Metadata != nil {
		for k, v := range metrics.Metadata {
			fields[k] = v
		}
	}

	logEntryWithContext(ctx, InfoLevel, fmt.Sprintf("Performance: %s.%s", metrics.Component, metrics.Operation), fields)
}

func LogSlowRequest(ctx context.Context, threshold time.Duration, data RequestLogData) {
	if data.Duration < threshold.Milliseconds() {
		return
	}

	fields := map[string]interface{}{
		"method":        data.Method,
		"path":          data.Path,
		"query":         data.Query,
		"ip":            data.IP,
		"status":        data.Status,
		"duration":      data.Duration,
		"threshold_ms":  threshold.Milliseconds(),
		"request_size":  data.RequestSize,
		"response_size": data.ResponseSize,
		"user_agent":    data.UserAgent,
	}

	logEntryWithContext(ctx, WarnLevel, "Slow request detected", fields)
}
