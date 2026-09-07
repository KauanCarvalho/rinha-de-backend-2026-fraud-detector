package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/config"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/detector"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/httpapi"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/observability"
)

const (
	exitOK  = 0
	exitErr = 1
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg := config.Load()
	logger := observability.NewLogger(cfg.LogLevel)

	idx, loadErr := knn.LoadFile(cfg.IndexPath)
	if loadErr != nil {
		logger.Error("failed to load index", "path", cfg.IndexPath, "error", loadErr)
		return exitErr
	}

	// gob-decoding a multi-million-entry index allocates and discards a lot
	// of scratch memory; returning it to the OS now (rather than waiting
	// for a GC cycle mid-traffic) keeps the container's first requests from
	// competing with that cleanup under the challenge's tight memory limit.
	debug.FreeOSMemory()

	logger.Info("index loaded", "path", cfg.IndexPath, "vectors", idx.Len())

	det := detector.New(idx, cfg.KNNMaxExtraLeaves)
	server := httpapi.NewServer(cfg, det, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			logger.Error("server error", "error", err)
			return exitErr
		}
		return exitOK
	}

	return shutdown(server, logger, cfg.ShutdownTimeout)
}

func shutdown(server *httpapi.Server, logger *slog.Logger, timeout time.Duration) int {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		return exitErr
	}
	logger.Info("server stopped")
	return exitOK
}
