package config

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultPort              = "9999"
	defaultIndexPath         = "data/index.bin"
	defaultLogLevel          = "info"
	defaultKNNMaxExtraLeaves = 1000
	defaultTimeout           = 5 * time.Second
)

type Config struct {
	Port              string
	IndexPath         string
	LogLevel          string
	KNNMaxExtraLeaves int
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	ShutdownTimeout   time.Duration
}

func Load() Config {
	return Config{
		Port:              getEnv("PORT", defaultPort),
		IndexPath:         getEnv("INDEX_PATH", defaultIndexPath),
		LogLevel:          getEnv("LOG_LEVEL", defaultLogLevel),
		KNNMaxExtraLeaves: getEnvInt("KNN_MAX_EXTRA_LEAVES", defaultKNNMaxExtraLeaves),
		ReadTimeout:       getEnvDuration("READ_TIMEOUT", defaultTimeout),
		WriteTimeout:      getEnvDuration("WRITE_TIMEOUT", defaultTimeout),
		ShutdownTimeout:   getEnvDuration("SHUTDOWN_TIMEOUT", defaultTimeout),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
