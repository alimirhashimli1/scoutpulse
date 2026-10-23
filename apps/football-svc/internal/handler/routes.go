package handler

import (
	"net/http"
	"strings"

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
	PlayerNote  *PlayerNoteHandler
	Season      *SeasonHandler
	CoachSpell  *CoachSpellHandler
	TeamEditor  *TeamEditorHandler
	TeamSeason  *TeamSeasonHandler
	Search      *SearchHandler
	Tactic      *TacticHandler
	Article     *ArticleHandler
}

// RegisterRoutes registers all API routes for the football service.
//
// Reads are public. Writes sit behind AuthMiddleware, which only proves the
// caller holds a valid token; which role may perform the write is decided in
// the service layer.
func RegisterRoutes(mux *http.ServeMux, h Handlers) {
	// Every authenticated write is limited per account. Notes and tactics
	// boards are open to any member, and accounts are free, so without this a
	// single script could fill the database as fast as it can send requests.
	// Keyed on the account rather than the address, so it holds however the
	// caller's IP is seen.
	writes := server.NewRateLimit(memberWriteRatePerSecond, memberWriteBurst)
	protect := func(pattern string, fn http.HandlerFunc) {
		var next http.Handler = fn
		if !strings.HasPrefix(pattern, http.MethodGet+" ") {
			next = writes.MiddlewareBy(accountKey, next)
		}
		mux.Handle(pattern, auth.AuthMiddleware(next))
	}

	mux.HandleFunc("GET /health", server.Health("Football Service"))

	// --- search ---
	// Public, because everything it returns is reachable through the public
	// entity endpoints anyway; requiring a token would only break the search
	// box for signed-out visitors.
	mux.HandleFunc("GET /api/v1/search", h.Search.Search)

	// --- competitions ---
	mux.HandleFunc("GET /api/v1/leagues", h.League.ListLeagues)
	mux.HandleFunc("GET /api/v1/leagues/{id}", h.League.GetLeague)
	protect("POST /api/v1/leagues", h.League.CreateLeague)
	protect("PUT /api/v1/leagues/{id}", h.League.UpdateLeague)
	protect("DELETE /api/v1/leagues/{id}", h.League.DeleteLeague)

	// --- seasons ---
	// "current" is registered before "{id}" so it is not read as an id.
	mux.HandleFunc("GET /api/v1/seasons/current", h.Season.GetCurrentSeason)
	mux.HandleFunc("GET /api/v1/seasons", h.Season.ListSeasons)
	mux.HandleFunc("GET /api/v1/seasons/{id}", h.Season.GetSeason)
	// Every club entered in this season; ?league_id= narrows it to one
	// competition, which is what a "who played in this league that year" page
	// needs.
	mux.HandleFunc("GET /api/v1/seasons/{id}/teams", h.TeamSeason.ListBySeason)
	protect("POST /api/v1/seasons", h.Season.CreateSeason)
	protect("PUT /api/v1/seasons/{id}", h.Season.UpdateSeason)
	protect("DELETE /api/v1/seasons/{id}", h.Season.DeleteSeason)

	// --- clubs ---
	// league_id is an optional filter on GET /api/v1/teams, not a requirement.
	mux.HandleFunc("GET /api/v1/teams", h.Team.ListTeams)
	mux.HandleFunc("GET /api/v1/teams/{id}", h.Team.GetTeam)
	mux.HandleFunc("GET /api/v1/teams/{id}/coaches", h.CoachSpell.ListByTeam)
	// Which competitions this club has contested, newest season first.
	mux.HandleFunc("GET /api/v1/teams/{id}/seasons", h.TeamSeason.ListByTeam)
	protect("POST /api/v1/teams/{id}/seasons", h.TeamSeason.Enter)
	protect("DELETE /api/v1/teams/{id}/seasons/{entryID}", h.TeamSeason.Withdraw)
	protect("POST /api/v1/teams", h.Team.CreateTeam)
	protect("PUT /api/v1/teams/{id}", h.Team.UpdateTeam)
	protect("DELETE /api/v1/teams/{id}", h.Team.DeleteTeam)

	// Editor grants. Administrative, so every route is authenticated --
	// including the reads, since who may edit a club is access-control data.
	protect("GET /api/v1/teams/{id}/editors", h.TeamEditor.ListEditors)
	protect("POST /api/v1/teams/{id}/editors", h.TeamEditor.Grant)
	protect("DELETE /api/v1/teams/{id}/editors/{userID}", h.TeamEditor.Revoke)
	protect("GET /api/v1/users/{userID}/teams", h.TeamEditor.ListMyTeams)

	// --- coaches ---
	// GET /api/v1/coaches requires a team_id query parameter.
	mux.HandleFunc("GET /api/v1/coaches", h.Coach.GetCoachByTeam)
	mux.HandleFunc("GET /api/v1/coaches/{id}", h.Coach.GetCoach)
	mux.HandleFunc("GET /api/v1/coaches/{id}/spells", h.CoachSpell.ListByCoach)
	protect("POST /api/v1/coaches", h.Coach.CreateCoach)
	protect("PUT /api/v1/coaches/{id}", h.Coach.UpdateCoach)
	protect("DELETE /api/v1/coaches/{id}", h.Coach.DeleteCoach)
	protect("POST /api/v1/coaches/{id}/spells", h.CoachSpell.RecordSpell)
	protect("DELETE /api/v1/coaches/{id}/spells/{spellID}", h.CoachSpell.DeleteSpell)

	// --- players ---
	mux.HandleFunc("GET /api/v1/players", h.Player.ListPlayers)
	mux.HandleFunc("GET /api/v1/players/{id}", h.Player.GetPlayer)
	mux.HandleFunc("GET /api/v1/players/{id}/transfers", h.Transfer.ListByPlayer)
	mux.HandleFunc("GET /api/v1/players/{id}/market-values", h.MarketValue.ListValues)
	protect("POST /api/v1/players", h.Player.CreatePlayer)
	protect("PUT /api/v1/players/{id}", h.Player.UpdatePlayer)
	protect("DELETE /api/v1/players/{id}", h.Player.DeletePlayer)
	protect("POST /api/v1/players/{id}/market-values", h.MarketValue.RecordValue)
	protect("DELETE /api/v1/players/{id}/market-values/{valueID}", h.MarketValue.DeleteValue)

	// Member notes. Reading is public -- the commentary is the product. Writing
	// needs only a signed-in account, unlike every other write in this service:
	// the point is a range of opinions, and one-note-per-person is what keeps
	// that from becoming a free-for-all.
	mux.HandleFunc("GET /api/v1/players/{id}/notes", h.PlayerNote.ListNotes)
	protect("GET /api/v1/players/{id}/notes/mine", h.PlayerNote.MyNote)
	protect("POST /api/v1/players/{id}/notes", h.PlayerNote.WriteNote)
	protect("PUT /api/v1/players/{id}/notes/{noteID}", h.PlayerNote.EditNote)
	protect("DELETE /api/v1/players/{id}/notes/{noteID}", h.PlayerNote.DeleteNote)

	// --- transfers ---
	// The transfer feed: the central read of a Transfermarkt-style product.
	mux.HandleFunc("GET /api/v1/transfers", h.Transfer.ListTransfers)
	mux.HandleFunc("GET /api/v1/transfers/{id}", h.Transfer.GetTransfer)
	protect("POST /api/v1/transfers", h.Transfer.RecordTransfer)
	// Corrects type, fee, currency and season. The clubs and the date are not
	// writable here -- they are what the current squad is derived from.
	protect("PUT /api/v1/transfers/{id}", h.Transfer.UpdateTransfer)
	protect("DELETE /api/v1/transfers/{id}", h.Transfer.DeleteTransfer)

	// --- tactics boards ---
	// Unlisted rather than private. One board is public by its slug, because
	// the players and coaches it is sent to have no accounts; the directory of
	// boards is for members only, which is what keeps them out of view. Any
	// signed-in member may create one, and each sees only their own in the list.
	mux.HandleFunc("GET /api/v1/tactics/{slug}", h.Tactic.GetTactic)
	protect("GET /api/v1/tactics", h.Tactic.ListTactics)
	protect("POST /api/v1/tactics", h.Tactic.CreateTactic)
	protect("PUT /api/v1/tactics/{slug}", h.Tactic.UpdateTactic)
	protect("DELETE /api/v1/tactics/{slug}", h.Tactic.DeleteTactic)

	// --- analysis articles ---
	// The section readers see is public and holds only live articles. Everything
	// else -- drafts, the review queue, scheduling -- is the editorial desk,
	// behind a token; which role may do what there is decided in the service.
	mux.HandleFunc("GET /api/v1/articles", h.Article.ListArticles)
	mux.HandleFunc("GET /api/v1/articles/{slug}", h.Article.GetArticle)
	protect("GET /api/v1/desk/articles", h.Article.DeskList)
	protect("POST /api/v1/desk/articles", h.Article.Create)
	protect("GET /api/v1/desk/articles/{id}", h.Article.DeskGet)
	protect("PUT /api/v1/desk/articles/{id}", h.Article.Update)
	protect("DELETE /api/v1/desk/articles/{id}", h.Article.Delete)
	protect("POST /api/v1/desk/articles/{id}/submit", h.Article.Submit)
	protect("POST /api/v1/desk/articles/{id}/request-changes", h.Article.RequestChanges)
	protect("POST /api/v1/desk/articles/{id}/withdraw", h.Article.Withdraw)
}

// Per-account write allowance: a burst of 60, then two a second. An editor
// working through a squad never notices it.
const (
	memberWriteRatePerSecond = 2.0
	memberWriteBurst         = 60
)

// accountKey buckets a request by the account that sent it. Only reached
// behind AuthMiddleware, so the claims are always present; the IP is the
// fallback if that ever stops being true.
func accountKey(r *http.Request) string {
	if claims, ok := auth.GetClaims(r.Context()); ok {
		return "user:" + claims.UserID
	}
	return "ip:" + server.ClientIP(r)
}
