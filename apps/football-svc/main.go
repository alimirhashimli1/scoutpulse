package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/scoutpulse/football-svc/internal/handler"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/db"
	"github.com/scoutpulse/libs/platform/events"
	"github.com/scoutpulse/libs/platform/observability"
	"github.com/scoutpulse/libs/platform/server"
)

const (
	serviceName = "football-svc"
	version     = "0.4.0"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Fail fast: without a verification key this service cannot validate any
	// token, so every protected route would reject every request.
	if err := auth.LoadFromEnv(); err != nil {
		log.Fatalf("Failed to configure token verification: %v", err)
	}

	ctx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	// When verification is backed by a JWKS URL, poll it so a key rotation at
	// the issuer propagates without redeploying this service. A no-op when a
	// public key was pinned through the environment instead.
	auth.StartJWKSRefresh(ctx, logger)

	database, err := db.ConnectFromEnv()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// The event bus is optional: without NATS_URL this is a no-op publisher
	// and the service runs normally, because events are a secondary effect of
	// a write rather than part of it.
	publisher, err := events.PublisherFromEnv(logger)
	if err != nil {
		log.Fatalf("Failed to connect to the event bus: %v", err)
	}
	// Labelled with the service name so events_published_total distinguishes
	// which service dropped an event.
	safePublisher := events.NewSafePublisherFor(serviceName, publisher, logger)
	defer func() { _ = publisher.Close() }()

	// Repositories
	leagueRepo := repository.NewPostgresLeagueRepository(database)
	teamRepo := repository.NewPostgresTeamRepository(database)
	coachRepo := repository.NewPostgresCoachRepository(database)
	playerRepo := repository.NewPostgresPlayerRepository(database)
	transferRepo := repository.NewPostgresTransferRepository(database)
	marketValueRepo := repository.NewPostgresMarketValueRepository(database)
	seasonRepo := repository.NewPostgresSeasonRepository(database)
	coachSpellRepo := repository.NewPostgresCoachSpellRepository(database)
	teamEditorRepo := repository.NewPostgresTeamEditorRepository(database)

	// Authorization: one instance, shared by every service, so the grant
	// cache is shared and an invalidation reaches all of them.
	authz := service.NewAuthorizer(teamEditorRepo)

	// Services
	leagueService := service.NewLeagueService(leagueRepo, authz)
	teamService := service.NewTeamService(teamRepo, authz, safePublisher)
	coachService := service.NewCoachService(coachRepo, authz)
	playerService := service.NewPlayerService(playerRepo, authz, safePublisher)
	seasonService := service.NewSeasonService(seasonRepo, authz)
	transferService := service.NewTransferService(transferRepo, playerRepo, seasonRepo, authz, safePublisher)
	marketValueService := service.NewMarketValueService(marketValueRepo, playerRepo, authz, safePublisher)
	coachSpellService := service.NewCoachSpellService(coachSpellRepo, coachRepo, authz, safePublisher)
	teamEditorService := service.NewTeamEditorService(teamEditorRepo, teamRepo, authz)

	handlers := handler.Handlers{
		League:      handler.NewLeagueHandler(leagueService),
		Team:        handler.NewTeamHandler(teamService, playerService, coachService),
		Coach:       handler.NewCoachHandler(coachService),
		Player:      handler.NewPlayerHandler(playerService),
		Transfer:    handler.NewTransferHandler(transferService),
		MarketValue: handler.NewMarketValueHandler(marketValueService),
		Season:      handler.NewSeasonHandler(seasonService),
		CoachSpell:  handler.NewCoachSpellHandler(coachSpellService),
		TeamEditor:  handler.NewTeamEditorHandler(teamEditorService),
	}

	// Tracing is optional: with no collector configured this installs a
	// no-op, because telemetry must never be a runtime dependency of serving
	// a request.
	shutdownTracing, err := observability.InitTracing(ctx, serviceName, version, logger)
	if err != nil {
		log.Fatalf("Failed to initialise tracing: %v", err)
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, handlers)
	mux.Handle("GET "+observability.MetricsPath, observability.Handler())

	err = server.RunWithOptions(serviceName, ":8081", mux, server.Options{
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
