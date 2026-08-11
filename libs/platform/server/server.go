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
	// ShutdownTimeout exceeds WriteTimeout deliberately. A request that is
	// still inside its own write budget must be allowed to finish during a
	// drain; a shorter shutdown window would cut off exactly the slow requests
	// the write timeout was sized for.
	ShutdownTimeout = WriteTimeout + 5*time.Second
)

// Options tunes the server. The zero value is valid and used by Run.
type Options struct {
	// AllowedOrigins for CORS. Defaults to the CORS_ALLOWED_ORIGINS
	// environment variable, or "*" when that is unset.
	AllowedOrigins []string
	// Logger receives request logs. Defaults to slog.Default().
	Logger *slog.Logger
	// Middleware is applied inside the standard chain, so metrics and
	// tracing see the request after a correlation id exists but before the
	// route handler runs. Supplied by callers so this package does not
	// depend on the observability stack.
	Middleware []Middleware
	// OnShutdown runs after in-flight requests drain, for flushing
	// telemetry and closing connections.
	OnShutdown []func(context.Context) error
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

	// Order matters. RequestID is outermost so every later layer can log the
	// correlation id.
	//
	// Recover appears twice, and deliberately. The outer copy sits directly
	// inside RequestID so a panic anywhere below -- including in the caller's
	// metrics or tracing middleware -- becomes a 500 rather than a dropped
	// connection. The inner copy sits closest to the route handler so that the
	// common case, a panic in application code, is still caught below the
	// observability layers and is therefore counted and traced like any other
	// 500 rather than bypassing them.
	chain := []Middleware{
		RequestID,
		Recover(logger),
		Logging(logger),
		CORS(origins),
	}
	chain = append(chain, opts.Middleware...)
	chain = append(chain, Recover(logger))

	handler := Chain(h, chain...)

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

	// Run after the server drains, so telemetry for the final requests is
	// flushed rather than discarded.
	for _, fn := range opts.OnShutdown {
		if err := fn(shutdownCtx); err != nil {
			logger.Error("shutdown hook failed", "service", name, "error", err)
		}
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
