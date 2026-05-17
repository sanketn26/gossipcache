package observability

import (
	"io"
	"log/slog"
	"os"
)

// Logger wraps slog for structured logging
type Logger struct {
	*slog.Logger
}

// NewLogger creates a new logger with the given configuration writing to stdout.
func NewLogger(level, format string) *Logger {
	return NewLoggerWithWriter(level, format, os.Stdout)
}

// NewLoggerWithWriter creates a logger that writes to w. Tests inject a buffer
// to capture log output.
func NewLoggerWithWriter(level, format string, w io.Writer) *Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: logLevel}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	return &Logger{Logger: slog.New(handler)}
}

// WithComponent returns a logger with a component field
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.With("component", component),
	}
}
