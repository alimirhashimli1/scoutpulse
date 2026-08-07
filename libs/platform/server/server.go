// Package server provides the HTTP bootstrap shared by every ScoutPulse
// service: hardened timeouts, a standard middleware chain, and graceful
// shutdown.
//
// A new service's main.go should not hand-roll any of this. Wire your routes,
// then hand the mux to Run.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Timeouts applied to every service. ReadHeaderTimeout in particular is what
// closes the Slowloris hole left by a bare http.ListenAndServe.
const (
	ReadHeaderTimeout = 5 * time.Second
	ReadTimeout       = 15 * time.Second
	WriteTimeout      = 30 * time.Second
	IdleTimeout       = 60 * time.Second
	ShutdownTimeout   = 15 * time.Second
)

// Options tunes the server. The zero value is valid and used by Run.
type Options struct {
	// AllowedOrigins for CORS. Defaults to the CORS_ALLOWED_ORIGINS
	// environment variable, or "*" when that is unset.
	AllowedOrigins []string
	// Logger receives request logs. Defaults to slog.Default().
	Logger *slog.Logger
}

// Run starts an HTTP server on addr with the standard middleware chain and
// blocks until the process receives SIGINT or SIGTERM, then drains in-flight
// requests before returning.
func Run(name, addr string, h http.Handler) error {
	return RunWithOptions(name, addr, h, Options{})
}

// RunWithOptions is Run with explicit configuration.
func RunWithOptions(name, addr string, h http.Handler, opts Options) error {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	origins := opts.AllowedOrigins
	if len(origins) == 0 {
		origins = allowedOriginsFromEnv()
	}

	handler := Chain(h,
		RequestID,
		Logging(logger),
		CORS(origins),
		Recover(logger),
	)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: ReadHeaderTimeout,
		ReadTimeout:       ReadTimeout,
		WriteTimeout:      WriteTimeout,
		IdleTimeout:       IdleTimeout,
	}

	// Shut down on the signals a container runtime actually sends.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "service", name, "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining", "service", name)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	logger.Info("server stopped cleanly", "service", name)
	return nil
}

// Health returns a handler for liveness checks.
func Health(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "%s is healthy", name)
	}
}
