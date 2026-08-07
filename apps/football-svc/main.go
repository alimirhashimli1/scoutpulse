package main

import (
	"log"
	"net/http"

	"github.com/scoutpulse/football-svc/internal/handler"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/db"
	"github.com/scoutpulse/libs/platform/server"
)

func main() {
	// Fail fast: without the signing key this service cannot validate any
	// token, so every protected route would reject every request.
	if err := auth.LoadSecretFromEnv(); err != nil {
		log.Fatalf("Failed to load JWT signing key: %v", err)
	}

	database, err := db.ConnectFromEnv()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Repositories
	leagueRepo := repository.NewPostgresLeagueRepository(database)
	teamRepo := repository.NewPostgresTeamRepository(database)
	coachRepo := repository.NewPostgresCoachRepository(database)
	playerRepo := repository.NewPostgresPlayerRepository(database)

	// Services
	leagueService := service.NewLeagueService(leagueRepo)
	teamService := service.NewTeamService(teamRepo)
	coachService := service.NewCoachService(coachRepo)
	playerService := service.NewPlayerService(playerRepo)

	// Handlers
	leagueHandler := handler.NewLeagueHandler(leagueService)
	teamHandler := handler.NewTeamHandler(teamService, playerService, coachService)
	coachHandler := handler.NewCoachHandler(coachService)
	playerHandler := handler.NewPlayerHandler(playerService)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, leagueHandler, teamHandler, coachHandler, playerHandler)

	if err := server.Run("football-svc", ":8081", mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
