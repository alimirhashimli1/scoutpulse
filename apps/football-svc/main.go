package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/scoutpulse/football-svc/internal/handler"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/db"
)

func main() {
	database, err := db.ConnectFromEnv()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Initialize Repositories
	leagueRepo := repository.NewPostgresLeagueRepository(database)
	teamRepo := repository.NewPostgresTeamRepository(database)
	coachRepo := repository.NewPostgresCoachRepository(database)
	playerRepo := repository.NewPostgresPlayerRepository(database)

	// Initialize Services
	leagueService := service.NewLeagueService(leagueRepo)
	teamService := service.NewTeamService(teamRepo)
	coachService := service.NewCoachService(coachRepo)
	playerService := service.NewPlayerService(playerRepo)

	// Initialize Handlers
	leagueHandler := handler.NewLeagueHandler(leagueService)
	teamHandler := handler.NewTeamHandler(teamService, playerService, coachService) // Pass playerService and coachService here
	coachHandler := handler.NewCoachHandler(coachService)
	playerHandler := handler.NewPlayerHandler(playerService)

	mux := http.NewServeMux()

	// Register all routes
	handler.RegisterRoutes(mux, leagueHandler, teamHandler, coachHandler, playerHandler)

	// Health check - always public
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Football Service is healthy"))
	})

	port := ":8081"
	fmt.Printf("Football Service starting on port %s\n", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
