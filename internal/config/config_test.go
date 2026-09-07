package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()

	cfg := config.Load()

	assert.Equal(t, "9999", cfg.Port)
	assert.Equal(t, "data/index.bin", cfg.IndexPath)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 5000, cfg.KNNMaxExtraLeaves)
	assert.Equal(t, 5*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 5*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 5*time.Second, cfg.ShutdownTimeout)
}

func TestLoad_OverridesFromEnv(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("INDEX_PATH", "/index/index.bin")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("KNN_MAX_EXTRA_LEAVES", "500")
	t.Setenv("READ_TIMEOUT", "2s")
	t.Setenv("WRITE_TIMEOUT", "3s")
	t.Setenv("SHUTDOWN_TIMEOUT", "1500ms")

	cfg := config.Load()

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "/index/index.bin", cfg.IndexPath)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 500, cfg.KNNMaxExtraLeaves)
	assert.Equal(t, 2*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 3*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 1500*time.Millisecond, cfg.ShutdownTimeout)
}

func TestLoad_InvalidNumericEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("KNN_MAX_EXTRA_LEAVES", "not-a-number")
	t.Setenv("READ_TIMEOUT", "not-a-duration")

	cfg := config.Load()

	assert.Equal(t, 5000, cfg.KNNMaxExtraLeaves)
	assert.Equal(t, 5*time.Second, cfg.ReadTimeout)
}
