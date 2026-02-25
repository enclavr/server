package logger

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

type Level string

const (
	DebugLevel Level = "DEBUG"
	InfoLevel  Level = "INFO"
	WarnLevel  Level = "WARN"
	ErrorLevel Level = "ERROR"
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
}

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

func SetOutput(w io.Writer) {
	output = w
	if defaultLogger != nil {
		defaultLogger.logger.SetOutput(w)
	}
}

func logEntry(level Level, msg string, fields map[string]interface{}) {
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
