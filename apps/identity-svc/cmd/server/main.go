package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/scoutpulse/identity-svc/internal/handler"
	"github.com/scoutpulse/identity-svc/internal/oauth"
	"github.com/scoutpulse/identity-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/db"
	"github.com/scoutpulse/libs/platform/events"
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

// How often the two janitors run. Login codes expire in 60 seconds and one is
// written per provider sign-in, so they are swept far more often than refresh
// tokens, which live 30 days.
const (
	sessionJanitorInterval   = time.Hour
	loginCodeJanitorInterval = 15 * time.Minute
)

// getenv reads a variable with a fallback, treating empty as unset — an
// exported-but-blank variable in a compose file is a mistake, not a choice.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// startLoginCodeJanitor prunes spent and expired OAuth login codes.
//
// A code lives 60 seconds, but its row does not disappear on its own — and one
// is written on every provider sign-in, so the table would grow with traffic
// exactly as refresh_tokens did before it had a janitor.
func startLoginCodeJanitor(ctx context.Context, repo repository.LoginCodeRepository, logger *slog.Logger) {
	go func() {
		ticker := time.NewTicker(loginCodeJanitorInterval)
		defer ticker.Stop()

		prune := func() {
			runCtx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()

			removed, err := repo.DeleteExpired(runCtx, time.Now())
			if err != nil {
				logger.Error("pruning sign-in codes failed", "error", err)
				return
			}
			if removed > 0 {
				logger.Debug("pruned sign-in codes", "count", removed)
			}
		}

		prune()
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

	// The event bus is optional: without NATS_URL this is a no-op publisher and
	// the service runs normally, because events are a secondary effect of a
	// write rather than part of it.
	publisher, err := events.PublisherFromEnv(logger)
	if err != nil {
		log.Fatalf("Failed to connect to the event bus: %v", err)
	}
	defer func() { _ = publisher.Close() }()

	userRepo := repository.NewPostgresUserRepository(database)
	refreshRepo := repository.NewPostgresRefreshTokenRepository(database)
	identityRepo := repository.NewPostgresIdentityRepository(database)
	loginCodeRepo := repository.NewPostgresLoginCodeRepository(database)

	// External sign-in providers. A provider with no credentials in the
	// environment is simply absent, so Google can be enabled without Facebook
	// and the password flow is unaffected either way.
	publicURL := getenv("PUBLIC_BASE_URL", "http://localhost:8080")
	frontendURL := getenv("FRONTEND_URL", "http://localhost:4200")
	providers := oauth.FromEnv(publicURL)
	if names := providers.Configured(); len(names) > 0 {
		logger.Info("external sign-in enabled", "providers", names)
	} else {
		logger.Info("no external sign-in providers configured")
	}

	h := &handler.Handler{
		UserRepo:    userRepo,
		RefreshRepo: refreshRepo,
		Publisher:   events.NewSafePublisherFor(serviceName, publisher, logger),
		OAuth: handler.OAuthDeps{
			Providers:   providers,
			Identities:  identityRepo,
			LoginCodes:  loginCodeRepo,
			FrontendURL: frontendURL,
			// The flow cookies are only marked Secure when the service is
			// actually reached over https; a Secure cookie is silently dropped
			// on plain http, which would break local development in a way that
			// looks like the state check failing.
			SecureCookies: strings.HasPrefix(publicURL, "https://"),
		},
	}

	// Expired and revoked sessions accumulate forever otherwise: rotation
	// writes a new row on every refresh, so one active user produces a row
	// every 15 minutes and nothing ever removes them.
	ctx, stopJanitor := context.WithCancel(context.Background())
	defer stopJanitor()
	startSessionJanitor(ctx, refreshRepo, logger)
	// Login codes live 60 seconds but the rows outlast them, so they need
	// pruning for the same reason refresh tokens do.
	startLoginCodeJanitor(ctx, loginCodeRepo, logger)

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

	// --- external sign-in ---
	//
	// The browser is sent to the provider and comes back here; the completed
	// sign-in is handed to the frontend as a one-time code, which it exchanges
	// for the usual token pair. Tokens never travel in a redirect URL.
	mux.HandleFunc("GET /api/v1/auth/providers", h.ListProviders)
	mux.HandleFunc("GET /api/v1/auth/{provider}", h.StartOAuth)
	mux.HandleFunc("GET /api/v1/auth/{provider}/callback", h.CompleteOAuth)
	mux.Handle("POST /api/v1/auth/exchange", writeLimit.Middleware(http.HandlerFunc(h.ExchangeCode)))

	// Which providers the caller has linked, and unlinking one.
	mux.Handle("GET /api/v1/users/me/identities",
		auth.AuthMiddleware(http.HandlerFunc(h.ListIdentities)))
	mux.Handle("DELETE /api/v1/users/me/identities/{provider}",
		auth.AuthMiddleware(http.HandlerFunc(h.UnlinkIdentity)))

	mux.Handle("GET /api/v1/users/me", auth.AuthMiddleware(http.HandlerFunc(h.Me)))
	// Rate limited like the other credential endpoints: the current password
	// is checked here, so this is another place to guess one.
	mux.Handle("PUT /api/v1/users/me/password",
		auth.AuthMiddleware(loginLimit.Middleware(http.HandlerFunc(h.ChangePassword))))

	// Administrative. Who holds an account is not public information.
	mux.Handle("GET /api/v1/users", auth.AuthMiddleware(http.HandlerFunc(h.ListUsers)))
	mux.Handle("PUT /api/v1/users/{id}/role", auth.AuthMiddleware(http.HandlerFunc(h.UpdateRole)))
	mux.Handle("DELETE /api/v1/users/{id}", auth.AuthMiddleware(http.HandlerFunc(h.DeleteUser)))

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
