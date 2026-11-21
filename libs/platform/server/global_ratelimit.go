package server

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Defaults for the service-wide, per-address allowance.
//
// Deliberately generous. This is a backstop against one address hammering the
// API -- a scraper, a runaway script, a crude flood -- not a fine-grained
// control; login, registration and writes keep their own much tighter limits.
// A real visitor, or a classroom of thirty behind one NAT address playing an
// online game, stays far below it.
const (
	DefaultRequestsPerMinute = 900
	DefaultRequestBurst      = 300
)

// GlobalRateLimitFromEnv builds the per-IP limit every service applies to
// every request, configured by:
//
//   - RATE_LIMIT_PER_MINUTE: sustained requests per minute per address.
//     "0" turns the limit off. Default 900.
//   - RATE_LIMIT_BURST: how many may arrive at once. Default 300.
//
// The address comes from ClientIP, so it follows CLIENT_IP_HEADER. If every
// request appears to come from one address (the client_ip field in the request
// log shows it), that address is a proxy's, every visitor shares one bucket,
// and the right fix is CLIENT_IP_HEADER -- or, failing that, turning this off
// and rate limiting at the edge instead.
//
// Returns nil when disabled.
func GlobalRateLimitFromEnv(logger *slog.Logger) Middleware {
	perMinute := envInt("RATE_LIMIT_PER_MINUTE", DefaultRequestsPerMinute)
	burst := envInt("RATE_LIMIT_BURST", DefaultRequestBurst)
	if perMinute <= 0 {
		logger.Info("global rate limit disabled")
		return nil
	}
	if burst <= 0 {
		burst = DefaultRequestBurst
	}
	logger.Info("global rate limit", "per_minute", perMinute, "burst", burst)
	return GlobalRateLimit(float64(perMinute)/60, burst)
}

// GlobalRateLimit limits every request per client address, except
// infrastructure probes and CORS preflights. A preflight is the browser's
// question, not the visitor's request, and refusing it would surface as a
// CORS error rather than a 429 the page can explain.
func GlobalRateLimit(ratePerSecond float64, burst int) Middleware {
	rl := NewRateLimit(ratePerSecond, burst)
	return func(next http.Handler) http.Handler {
		limited := rl.Middleware(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions || probePaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}

func envInt(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
