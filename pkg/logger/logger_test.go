package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestLogger_Init(t *testing.T) {
	buf := &bytes.Buffer{}
	SetOutput(buf)

	Init()

	if defaultLogger == nil {
		t.Fatal("defaultLogger should be initialized")
	}
}

func TestLogger_SetOutput(t *testing.T) {
	buf := &bytes.Buffer{}
	SetOutput(buf)

	if defaultLogger == nil {
		t.Fatal("defaultLogger should be initialized")
	}

	if defaultLogger.logger == nil {
		t.Fatal("logger should not be nil")
	}
}

func TestLogger_Debug(t *testing.T) {
	buf := &bytes.Buffer{}
	SetOutput(buf)

	Debug("test message", map[string]interface{}{"key": "value"})

	output := buf.String()
	if !strings.Contains(output, "DEBUG") {
		t.Errorf("expected DEBUG level in output, got: %s", output)
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("expected message in output, got: %s", output)
	}
	if !strings.Contains(output, "key") {
		t.Errorf("expected field key in output, got: %s", output)
	}
}

func TestLogger_Info(t *testing.T) {
	buf := &bytes.Buffer{}
	SetOutput(buf)

	Info("test info message", nil)

	output := buf.String()
	if !strings.Contains(output, "INFO") {
		t.Errorf("expected INFO level in output, got: %s", output)
	}
}

func TestLogger_Warn(t *testing.T) {
	buf := &bytes.Buffer{}
	SetOutput(buf)

	Warn("test warn message", nil)

	output := buf.String()
	if !strings.Contains(output, "WARN") {
		t.Errorf("expected WARN level in output, got: %s", output)
	}
}

func TestLogger_Error(t *testing.T) {
	buf := &bytes.Buffer{}
	SetOutput(buf)

	Error("test error message", nil)

	output := buf.String()
	if !strings.Contains(output, "ERROR") {
		t.Errorf("expected ERROR level in output, got: %s", output)
	}
}

func TestLogger_LogEntry(t *testing.T) {
	buf := &bytes.Buffer{}
	SetOutput(buf)

	logEntry(InfoLevel, "test log entry", map[string]interface{}{
		"foo": "bar",
		"num": 123,
	})

	output := buf.String()
	if !strings.Contains(output, "test log entry") {
		t.Errorf("expected message in output, got: %s", output)
	}
	if !strings.Contains(output, "foo") {
		t.Errorf("expected field foo in output, got: %s", output)
	}
	if !strings.Contains(output, "bar") {
		t.Errorf("expected field value in output, got: %s", output)
	}
}

func TestLogger_Concurrent(t *testing.T) {
	buf := &bytes.Buffer{}
	SetOutput(buf)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			Debug("concurrent message", map[string]interface{}{"n": n})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
