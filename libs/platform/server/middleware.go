package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"
)

// Middleware wraps a handler with additional behaviour.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware to h. The first entry is the outermost wrapper, so
// it sees the request first and the response last.
func Chain(h http.Handler, mw ...Middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

type contextKey string

const requestIDContextKey contextKey = "request_id"

// RequestIDHeader carries the correlation ID across service boundaries.
const RequestIDHeader = "X-Request-ID"

// RequestID attaches a correlation ID to the request context and echoes it in
// the response. An inbound X-Request-ID is preserved so a call chain across
// several services shares one ID.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = newRequestID()
		}

		w.Header().Set(RequestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDContextKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFrom returns the correlation ID attached by RequestID, or "" if the
// middleware did not run.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey).(string)
	return id
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// A correlation ID is diagnostic, not security-bearing; a timestamp
		// is an acceptable fallback if the entropy source is unavailable.
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}

// statusRecorder captures the status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// probePaths are hit on a fixed interval by infrastructure rather than by a
// user, so a successful one carries no information.
//
// Docker healthchecks poll /health every few seconds and Prometheus scrapes
// /metrics, which between them produce thousands of identical lines a day and
// bury the requests somebody actually wants to read. A *failing* probe is
// worth knowing about, so only the successful ones are dropped.
var probePaths = map[string]bool{
	"/health":  true,
	"/metrics": true,
}

// Logging emits one structured line per request, except for successful
// infrastructure probes.
func Logging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(rec, r)

			if rec.status == 0 {
				rec.status = http.StatusOK
			}

			if probePaths[r.URL.Path] && rec.status < http.StatusBadRequest {
				return
			}

			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", RequestIDFrom(r.Context())),
			)
		})
	}
}

// Recover turns a panic into a 500 instead of tearing down the connection,
// and logs the failure with its correlation ID.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.LogAttrs(r.Context(), slog.LevelError, "panic recovered",
						slog.Any("panic", rec),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.String("request_id", RequestIDFrom(r.Context())),
						slog.String("stack", string(debug.Stack())),
					)

					// JSON, matching every other error this platform emits, so
					// a client needs one error parser rather than a special
					// case for panics. The stack goes to the log only.
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error":      "internal server error",
						"code":       "internal",
						"request_id": RequestIDFrom(r.Context()),
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORSOriginsEnvVar configures allowed origins as a comma-separated list.
const CORSOriginsEnvVar = "CORS_ALLOWED_ORIGINS"

// allowedOriginsFromEnv reads the allowed origins.
//
// An unset variable yields no origins, not "*". A deployment that forgets to
// configure CORS should refuse cross-origin requests rather than accept every
// one of them: the failure has to be visible in the browser console, not
// silently permissive. The wildcard is still available, but it has to be
// spelled out.
func allowedOriginsFromEnv() []string {
	raw := os.Getenv(CORSOriginsEnvVar)

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}

// CORS answers preflight requests and sets the response headers a browser
// needs before it will hand a cross-origin response to JavaScript.
func CORS(allowedOrigins []string) Middleware {
	allowAll := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"

	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					// The response varies by Origin, so caches must not
					// serve one origin's response to another.
					w.Header().Add("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, "+RequestIDHeader)
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
