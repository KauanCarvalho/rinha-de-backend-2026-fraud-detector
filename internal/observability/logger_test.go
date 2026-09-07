package observability_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/observability"
)

func TestNewLogger_Level(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		level       string
		wantEnabled map[slog.Level]bool
	}{
		{
			name:  "debug enables everything",
			level: "debug",
			wantEnabled: map[slog.Level]bool{
				slog.LevelDebug: true, slog.LevelInfo: true, slog.LevelWarn: true, slog.LevelError: true,
			},
		},
		{
			name:  "info is the default",
			level: "info",
			wantEnabled: map[slog.Level]bool{
				slog.LevelDebug: false, slog.LevelInfo: true, slog.LevelWarn: true, slog.LevelError: true,
			},
		},
		{
			name:  "unknown level falls back to info",
			level: "bogus",
			wantEnabled: map[slog.Level]bool{
				slog.LevelDebug: false, slog.LevelInfo: true, slog.LevelWarn: true, slog.LevelError: true,
			},
		},
		{
			name:  "warn hides info and debug",
			level: "warn",
			wantEnabled: map[slog.Level]bool{
				slog.LevelDebug: false, slog.LevelInfo: false, slog.LevelWarn: true, slog.LevelError: true,
			},
		},
		{
			name:  "error only",
			level: "error",
			wantEnabled: map[slog.Level]bool{
				slog.LevelDebug: false, slog.LevelInfo: false, slog.LevelWarn: false, slog.LevelError: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := observability.NewLogger(tt.level)

			for lvl, want := range tt.wantEnabled {
				assert.Equalf(t, want, logger.Enabled(context.Background(), lvl), "level %s enabled=%v", lvl, lvl)
			}
		})
	}
}
