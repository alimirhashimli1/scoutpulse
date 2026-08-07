package handler

import (
	"net/http"

	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/server"
)

// RegisterRoutes registers all API routes for the football service.
//
// Reads are public. Writes sit behind AuthMiddleware, which only proves the
// caller holds a valid token — which role may perform the write is decided in
// the service layer.
func RegisterRoutes(
	mux *http.ServeMux,
	leagueHandler *LeagueHandler,
	teamHandler *TeamHandler,
	coachHandler *CoachHandler,
	playerHandler *PlayerHandler,
) {
	mux.HandleFunc("GET /health", server.Health("Football Service"))

	// Leagues
	mux.HandleFunc("GET /api/v1/leagues", leagueHandler.ListLeagues)
	mux.HandleFunc("GET /api/v1/leagues/{id}", leagueHandler.GetLeague)
	protect(mux, "POST /api/v1/leagues", leagueHandler.CreateLeague)
	protect(mux, "PUT /api/v1/leagues/{id}", leagueHandler.UpdateLeague)
	protect(mux, "DELETE /api/v1/leagues/{id}", leagueHandler.DeleteLeague)

	// Teams. GET /api/v1/teams requires a league_id query parameter.
	mux.HandleFunc("GET /api/v1/teams", teamHandler.ListTeams)
	mux.HandleFunc("GET /api/v1/teams/{id}", teamHandler.GetTeam)
	protect(mux, "POST /api/v1/teams", teamHandler.CreateTeam)
	protect(mux, "PUT /api/v1/teams/{id}", teamHandler.UpdateTeam)
	protect(mux, "DELETE /api/v1/teams/{id}", teamHandler.DeleteTeam)

	// Coaches. GET /api/v1/coaches requires a team_id query parameter.
	mux.HandleFunc("GET /api/v1/coaches", coachHandler.GetCoachByTeam)
	protect(mux, "POST /api/v1/coaches", coachHandler.CreateCoach)
	protect(mux, "PUT /api/v1/coaches/{id}", coachHandler.UpdateCoach)
	protect(mux, "DELETE /api/v1/coaches/{id}", coachHandler.DeleteCoach)

	// Players
	mux.HandleFunc("GET /api/v1/players", playerHandler.ListPlayers)
	mux.HandleFunc("GET /api/v1/players/{id}", playerHandler.GetPlayer)
	protect(mux, "POST /api/v1/players", playerHandler.CreatePlayer)
	protect(mux, "PUT /api/v1/players/{id}", playerHandler.UpdatePlayer)
	protect(mux, "DELETE /api/v1/players/{id}", playerHandler.DeletePlayer)
}

// protect registers a route that requires a valid bearer token.
func protect(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	mux.Handle(pattern, auth.AuthMiddleware(h))
}
