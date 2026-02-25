package profiler

import (
	"context"
	"errors"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* handlers on DefaultServeMux
	"time"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/config"
)

const (
	// DefaultShutdownTimeout is the timeout for graceful shutdown of the pprof server.
	DefaultShutdownTimeout = 5 * time.Second
	// DefaultReadHeaderTimeout is the timeout for reading request headers to prevent Slowloris attacks.
	DefaultReadHeaderTimeout = 5 * time.Second
)

// Server manages the pprof server lifecycle.
type Server struct {
	server *http.Server
}

// NewServer creates a new profiler server instance.
func NewServer() *Server {
	return &Server{}
}

// StartIfEnabled checks if profiler is enabled and starts the pprof server.
func (s *Server) StartIfEnabled(ctx context.Context, cfg config.ConfigurationProfiler) error {
	if !cfg.ProfilerEnabled() {
		// Profiler is explicitly disabled
		return nil
	}

	log := util.Log(ctx)
	profilerPort := cfg.ProfilerPort()
	log.WithField("port", profilerPort).Info("Starting pprof server")

	// Create pprof server
	s.server = &http.Server{
		Addr:              profilerPort,
		Handler:           nil, // Use default handler which includes pprof endpoints
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
	}

	// Start pprof server in a goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.WithError(err).Error("pprof server failed")
		}
	}()

	return nil
}

// Stop gracefully shuts down the pprof server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	log := util.Log(ctx)
	log.Info("stopping pprof server")

	shutdownCtx, cancel := context.WithTimeout(ctx, DefaultShutdownTimeout)
	defer cancel()

	if err := s.server.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("failed to shutdown pprof server")
		return err
	}

	s.server = nil
	return nil
}

// IsRunning returns true if the profiler server is currently running.
func (s *Server) IsRunning() bool {
	return s.server != nil
}
