package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/config"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/detector"
)

type Server struct {
	httpServer *http.Server
}

func NewHandler(det *detector.Detector, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", readyHandler)
	mux.HandleFunc("POST /fraud-score", fraudScoreHandler(det))

	var handler http.Handler = mux
	handler = recoverMiddleware(logger, handler)
	handler = loggingMiddleware(logger, handler)
	return handler
}

func NewServer(cfg config.Config, det *detector.Detector, logger *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         ":" + cfg.Port,
			Handler:      NewHandler(det, logger),
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
	}
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
