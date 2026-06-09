package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/scoutpulse/football-svc/internal/handler"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/auth"
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

	// Initialize Services
	leagueService := service.NewLeagueService(leagueRepo)
	teamService := service.NewTeamService(teamRepo)
	coachService := service.NewCoachService(coachRepo)

	// Initialize Handlers
	leagueHandler := handler.NewLeagueHandler(leagueService)
	teamHandler := handler.NewTeamHandler(teamService)
	coachHandler := handler.NewCoachHandler(coachService)

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Football Service is healthy"))
	})

	// League routes
	mux.HandleFunc("GET /leagues", leagueHandler.ListLeagues)
	mux.Handle("POST /leagues", auth.AuthMiddleware(http.HandlerFunc(leagueHandler.CreateLeague)))

	// Team routes
	mux.HandleFunc("GET /teams", teamHandler.ListTeams)
	mux.Handle("POST /teams", auth.AuthMiddleware(http.HandlerFunc(teamHandler.CreateTeam)))
	mux.Handle("PUT /teams/{id}", auth.AuthMiddleware(http.HandlerFunc(teamHandler.UpdateTeam)))

	// Coach routes
	mux.HandleFunc("GET /coaches", coachHandler.GetCoachByTeam)
	mux.Handle("POST /coaches", auth.AuthMiddleware(http.HandlerFunc(coachHandler.CreateCoach)))

	port := ":8081"
	fmt.Printf("Football Service starting on port %s\n", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
