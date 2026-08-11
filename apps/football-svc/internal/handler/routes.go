package handler

import (
	"net/http"

	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/server"
)

// Handlers collects every handler the router needs.
//
// A struct rather than a long parameter list: adding an entity should not mean
// touching every call site, and named fields make a miswired dependency a
// compile error instead of a silent argument swap.
type Handlers struct {
	League      *LeagueHandler
	Team        *TeamHandler
	Coach       *CoachHandler
	Player      *PlayerHandler
	Transfer    *TransferHandler
	MarketValue *MarketValueHandler
	Season      *SeasonHandler
	CoachSpell  *CoachSpellHandler
	TeamEditor  *TeamEditorHandler
}

// RegisterRoutes registers all API routes for the football service.
//
// Reads are public. Writes sit behind AuthMiddleware, which only proves the
// caller holds a valid token; which role may perform the write is decided in
// the service layer.
func RegisterRoutes(mux *http.ServeMux, h Handlers) {
	mux.HandleFunc("GET /health", server.Health("Football Service"))

	// --- competitions ---
	mux.HandleFunc("GET /api/v1/leagues", h.League.ListLeagues)
	mux.HandleFunc("GET /api/v1/leagues/{id}", h.League.GetLeague)
	protect(mux, "POST /api/v1/leagues", h.League.CreateLeague)
	protect(mux, "PUT /api/v1/leagues/{id}", h.League.UpdateLeague)
	protect(mux, "DELETE /api/v1/leagues/{id}", h.League.DeleteLeague)

	// --- seasons ---
	// "current" is registered before "{id}" so it is not read as an id.
	mux.HandleFunc("GET /api/v1/seasons/current", h.Season.GetCurrentSeason)
	mux.HandleFunc("GET /api/v1/seasons", h.Season.ListSeasons)
	mux.HandleFunc("GET /api/v1/seasons/{id}", h.Season.GetSeason)
	protect(mux, "POST /api/v1/seasons", h.Season.CreateSeason)
	protect(mux, "PUT /api/v1/seasons/{id}", h.Season.UpdateSeason)
	protect(mux, "DELETE /api/v1/seasons/{id}", h.Season.DeleteSeason)

	// --- clubs ---
	// GET /api/v1/teams requires a league_id query parameter.
	mux.HandleFunc("GET /api/v1/teams", h.Team.ListTeams)
	mux.HandleFunc("GET /api/v1/teams/{id}", h.Team.GetTeam)
	mux.HandleFunc("GET /api/v1/teams/{id}/coaches", h.CoachSpell.ListByTeam)
	protect(mux, "POST /api/v1/teams", h.Team.CreateTeam)
	protect(mux, "PUT /api/v1/teams/{id}", h.Team.UpdateTeam)
	protect(mux, "DELETE /api/v1/teams/{id}", h.Team.DeleteTeam)

	// Editor grants. Administrative, so every route is authenticated --
	// including the reads, since who may edit a club is access-control data.
	protect(mux, "GET /api/v1/teams/{id}/editors", h.TeamEditor.ListEditors)
	protect(mux, "POST /api/v1/teams/{id}/editors", h.TeamEditor.Grant)
	protect(mux, "DELETE /api/v1/teams/{id}/editors/{userID}", h.TeamEditor.Revoke)
	protect(mux, "GET /api/v1/users/{userID}/teams", h.TeamEditor.ListMyTeams)

	// --- coaches ---
	// GET /api/v1/coaches requires a team_id query parameter.
	mux.HandleFunc("GET /api/v1/coaches", h.Coach.GetCoachByTeam)
	mux.HandleFunc("GET /api/v1/coaches/{id}/spells", h.CoachSpell.ListByCoach)
	protect(mux, "POST /api/v1/coaches", h.Coach.CreateCoach)
	protect(mux, "PUT /api/v1/coaches/{id}", h.Coach.UpdateCoach)
	protect(mux, "DELETE /api/v1/coaches/{id}", h.Coach.DeleteCoach)
	protect(mux, "POST /api/v1/coaches/{id}/spells", h.CoachSpell.RecordSpell)
	protect(mux, "DELETE /api/v1/coaches/{id}/spells/{spellID}", h.CoachSpell.DeleteSpell)

	// --- players ---
	mux.HandleFunc("GET /api/v1/players", h.Player.ListPlayers)
	mux.HandleFunc("GET /api/v1/players/{id}", h.Player.GetPlayer)
	mux.HandleFunc("GET /api/v1/players/{id}/transfers", h.Transfer.ListByPlayer)
	mux.HandleFunc("GET /api/v1/players/{id}/market-values", h.MarketValue.ListValues)
	protect(mux, "POST /api/v1/players", h.Player.CreatePlayer)
	protect(mux, "PUT /api/v1/players/{id}", h.Player.UpdatePlayer)
	protect(mux, "DELETE /api/v1/players/{id}", h.Player.DeletePlayer)
	protect(mux, "POST /api/v1/players/{id}/market-values", h.MarketValue.RecordValue)
	protect(mux, "DELETE /api/v1/players/{id}/market-values/{valueID}", h.MarketValue.DeleteValue)

	// --- transfers ---
	// The transfer feed: the central read of a Transfermarkt-style product.
	mux.HandleFunc("GET /api/v1/transfers", h.Transfer.ListTransfers)
	mux.HandleFunc("GET /api/v1/transfers/{id}", h.Transfer.GetTransfer)
	protect(mux, "POST /api/v1/transfers", h.Transfer.RecordTransfer)
	protect(mux, "DELETE /api/v1/transfers/{id}", h.Transfer.DeleteTransfer)
}

// protect registers a route that requires a valid bearer token.
func protect(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	mux.Handle(pattern, auth.AuthMiddleware(h))
}
