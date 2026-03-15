package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// New creates a slog.Logger writing to file (or stdout if file is "").
// level: debug | info | warn | error
// format: text | json
func New(level, format, file string) *slog.Logger {
	var w io.Writer = os.Stdout
	if file != "" {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			// Fall back to stdout and warn — don't silently lose logs
			fmt.Fprintf(os.Stderr, "warning: cannot open log file %s: %v, using stdout\n", file, err)
		} else {
			w = f
		}
	}

	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}
	if strings.ToLower(format) == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}
