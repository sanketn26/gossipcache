package observability

import (
	"context"
	"log/slog"
	"testing"
)

func TestNewLoggerSupportsLevelsAndFormats(t *testing.T) {
	tests := []struct {
		name        string
		level       string
		format      string
		debugWanted bool
		infoWanted  bool
	}{
		{name: "debug json", level: "debug", format: "json", debugWanted: true, infoWanted: true},
		{name: "warn text", level: "warn", format: "text", debugWanted: false, infoWanted: false},
		{name: "unknown defaults to info", level: "verbose", format: "unknown", debugWanted: false, infoWanted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(tt.level, tt.format)
			if logger == nil || logger.Logger == nil {
				t.Fatal("NewLogger returned nil logger")
			}

			if got := logger.Enabled(context.Background(), slog.LevelDebug); got != tt.debugWanted {
				t.Fatalf("debug enabled = %t, want %t", got, tt.debugWanted)
			}
			if got := logger.Enabled(context.Background(), slog.LevelInfo); got != tt.infoWanted {
				t.Fatalf("info enabled = %t, want %t", got, tt.infoWanted)
			}
		})
	}
}

func TestLoggerWithComponent(t *testing.T) {
	logger := NewLogger("info", "json").WithComponent("cache")
	if logger == nil || logger.Logger == nil {
		t.Fatal("WithComponent returned nil logger")
	}
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("component logger should preserve info level")
	}
}
