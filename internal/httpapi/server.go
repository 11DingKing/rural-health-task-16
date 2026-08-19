package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"ruralhealth/internal/config"
	"ruralhealth/internal/logging"
)

// Server wraps the HTTP server with graceful shutdown support.
type Server struct {
	httpServer *http.Server
	logger     logging.Logger
}

func NewServer(cfg config.ServerConfig, handler http.Handler, logger logging.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.HTTPAddr,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
		logger: logger,
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.logger.Info(ctx, "http server starting", "addr", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context, timeout time.Duration) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	s.logger.Info(ctx, "http server shutting down")
	return s.httpServer.Shutdown(shutdownCtx)
}
