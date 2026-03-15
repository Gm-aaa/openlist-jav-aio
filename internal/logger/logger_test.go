package logger_test

import (
	"testing"
	"github.com/openlist-jav-aio/jav-aio/internal/logger"
)

func TestNewLogger_Debug(t *testing.T) {
	l := logger.New("debug", "text", "")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLogger_JSON(t *testing.T) {
	l := logger.New("info", "json", "")
	if l == nil {
		t.Fatal("expected non-nil logger for json format")
	}
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	// should fall back to info without panic
	l := logger.New("invalid", "text", "")
	if l == nil {
		t.Fatal("expected non-nil logger for invalid level")
	}
}
