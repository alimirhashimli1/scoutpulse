package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/scoutpulse/identity-svc/internal/handler"
	"github.com/scoutpulse/identity-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/db"
	"github.com/scoutpulse/libs/platform/observability"
	"github.com/scoutpulse/libs/platform/server"
)

const (
	serviceName = "identity-svc"
	version     = "0.4.0"
)

// Rate limits for the credential endpoints, per client.
//
// Login allows a short burst so a user fumbling their password is not locked
// out, then settles to a rate that makes bulk guessing impractical: 0.2/s is
// twelve attempts a minute sustained.
const (
	loginRatePerSecond = 0.2
	loginBurst         = 5
	writeRatePerSecond = 1.0
	writeBurst         = 10
)

// sessionJanitorInterval is how often expired refresh tokens are pruned.
const sessionJanitorInterval = time.Hour

// startSessionJanitor deletes expired refresh tokens on an interval.
//
// DeleteExpired existed, was on the interface, and was stubbed in the test
// fake -- but nothing called it, so the table only ever grew.
func startSessionJanitor(ctx context.Context, repo repository.RefreshTokenRepository, logger *slog.Logger) {
	go func() {
		ticker := time.NewTicker(sessionJanitorInterval)
		defer ticker.Stop()

		prune := func() {
			// Bounded so a slow delete cannot pile up behind the next tick.
			runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			removed, err := repo.DeleteExpired(runCtx, time.Now())
			if err != nil {
				logger.Error("pruning expired sessions failed", "error", err)
				return
			}
			if removed > 0 {
				logger.Info("pruned expired sessions", "count", removed)
			}
		}

		prune() // once at startup, so a long-running deployment is not the only trigger
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				prune()
			}
		}
	}()
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// This is the only service given the private key: it is the token issuer.
	// Every other service verifies with the public key published at
	// /.well-known/jwks.json and therefore cannot mint tokens of its own.
	if err := auth.LoadSignerFromEnv(); err != nil {
		log.Fatalf("Failed to load JWT signing key: %v", err)
	}

	database, err := db.ConnectFromEnv()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	// Closing on the way out of main: there is nothing left to report the
	// failure to, so the error is discarded explicitly rather than implicitly.
	defer func() { _ = database.Close() }()

	userRepo := repository.NewPostgresUserRepository(database)
	refreshRepo := repository.NewPostgresRefreshTokenRepository(database)
	h := &handler.Handler{UserRepo: userRepo, RefreshRepo: refreshRepo}

	// Expired and revoked sessions accumulate forever otherwise: rotation
	// writes a new row on every refresh, so one active user produces a row
	// every 15 minutes and nothing ever removes them.
	ctx, stopJanitor := context.WithCancel(context.Background())
	defer stopJanitor()
	startSessionJanitor(ctx, refreshRepo, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.Health)

	// Public key distribution. Deliberately unauthenticated: a JWKS document
	// contains only public material and every verifier needs to read it.
	mux.HandleFunc("GET "+auth.JWKSPath, auth.JWKSHandler())

	// Credential endpoints are rate limited per client.
	//
	// Login is the target that matters: unthrottled it gives an attacker
	// unlimited guesses, and each guess costs a bcrypt hash of server CPU
	// whether or not the account exists, so it is a cheap denial-of-service
	// vector as well as a credential-stuffing one. Register and refresh are
	// limited for the same reason at a looser rate.
	loginLimit := server.NewRateLimit(loginRatePerSecond, loginBurst)
	writeLimit := server.NewRateLimit(writeRatePerSecond, writeBurst)

	mux.Handle("POST /api/v1/auth/register", writeLimit.Middleware(http.HandlerFunc(h.Register)))
	mux.Handle("POST /api/v1/auth/login", loginLimit.Middleware(http.HandlerFunc(h.Login)))
	mux.Handle("POST /api/v1/auth/refresh", writeLimit.Middleware(http.HandlerFunc(h.Refresh)))
	mux.HandleFunc("POST /api/v1/auth/logout", h.Logout)

	mux.Handle("GET /api/v1/users/me", auth.AuthMiddleware(http.HandlerFunc(h.Me)))
	mux.Handle("PUT /api/v1/users/{id}/role", auth.AuthMiddleware(http.HandlerFunc(h.UpdateRole)))

	mux.Handle("GET "+observability.MetricsPath, observability.Handler())

	// Tracing is optional: with no collector configured this installs a
	// no-op, because telemetry must never be a runtime dependency of serving
	// a request.
	shutdownTracing, err := observability.InitTracing(context.Background(), serviceName, version, logger)
	if err != nil {
		log.Fatalf("Failed to initialise tracing: %v", err)
	}

	err = server.RunWithOptions(serviceName, ":8080", mux, server.Options{
		Logger: logger,
		Middleware: []server.Middleware{
			observability.Tracing(serviceName),
			observability.Metrics(serviceName),
		},
		OnShutdown: []func(context.Context) error{shutdownTracing},
	})
	if err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
