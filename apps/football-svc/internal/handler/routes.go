package handler

import (
	"net/http"

	"github.com/scoutpulse/libs/auth"
)

// LeagueService defines the interface for league-related operations.
// It's used here to avoid direct dependency on the concrete service implementation
// and facilitate mocking in tests.
type LeagueService interface {
	ListLeagues(r *http.Request)
	CreateLeague(w http.ResponseWriter, r *http.Request)
}

type TeamService interface {
	ListTeams(w http.ResponseWriter, r *http.Request)
	GetTeam(w http.ResponseWriter, r *http.Request)
	CreateTeam(w http.ResponseWriter, r *http.Request)
	UpdateTeam(w http.ResponseWriter, r *http.Request)
}

type CoachService interface {
	GetCoachByTeam(w http.ResponseWriter, r *http.Request)
	CreateCoach(w http.ResponseWriter, r *http.Request)
}

type PlayerService interface {
	ListPlayers(w http.ResponseWriter, r *http.Request)
	CreatePlayer(w http.ResponseWriter, r *http.Request)
	UpdatePlayer(w http.ResponseWriter, r *http.Request)
}

// RegisterRoutes registers all API routes for the football service.
func RegisterRoutes(
	mux *http.ServeMux,
	leagueHandler *LeagueHandler,
	teamHandler *TeamHandler,
	coachHandler *CoachHandler,
	playerHandler *PlayerHandler,
) {
	// League routes
	mux.HandleFunc("GET /api/v1/leagues", leagueHandler.ListLeagues)
	mux.Handle("POST /api/v1/leagues", auth.AuthMiddleware(http.HandlerFunc(leagueHandler.CreateLeague)))

	// Team routes
	mux.HandleFunc("GET /api/v1/teams", teamHandler.ListTeams) // This expects league_id query param
	mux.HandleFunc("GET /api/v1/teams/{id}", teamHandler.GetTeam)
	mux.Handle("POST /api/v1/teams", auth.AuthMiddleware(http.HandlerFunc(teamHandler.CreateTeam)))
	mux.Handle("PUT /api/v1/teams/{id}", auth.AuthMiddleware(http.HandlerFunc(teamHandler.UpdateTeam)))

	// Coach routes
	mux.HandleFunc("GET /api/v1/coaches", coachHandler.GetCoachByTeam) // This expects team_id query param
	mux.Handle("POST /api/v1/coaches", auth.AuthMiddleware(http.HandlerFunc(coachHandler.CreateCoach)))

	// Player routes
	mux.HandleFunc("GET /api/v1/players", playerHandler.ListPlayers)
	mux.Handle("POST /api/v1/players", auth.AuthMiddleware(http.HandlerFunc(playerHandler.CreatePlayer)))
	mux.Handle("PUT /api/v1/players/{id}", auth.AuthMiddleware(http.HandlerFunc(playerHandler.UpdatePlayer)))
}
