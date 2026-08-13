package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/httpx"
)

// TeamSeasonHandler serves which competitions a club contested, and when.
type TeamSeasonHandler struct {
	service service.TeamSeasonService
}

func NewTeamSeasonHandler(s service.TeamSeasonService) *TeamSeasonHandler {
	return &TeamSeasonHandler{service: s}
}

// ListByTeam serves a club's competition history, newest season first.
func (h *TeamSeasonHandler) ListByTeam(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	entries, err := h.service.ListByTeam(r.Context(), r.PathValue("id"), page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(entries, page))
}

// ListBySeason serves every club entered in a season, optionally narrowed to
// one competition.
func (h *TeamSeasonHandler) ListBySeason(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	entries, err := h.service.ListBySeason(
		r.Context(), r.PathValue("id"), httpx.QueryString(r, "league_id"), page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(entries, page))
}

// Enter records a club in a competition for a season.
func (h *TeamSeasonHandler) Enter(w http.ResponseWriter, r *http.Request) {
	var entry domain.TeamSeason
	if err := httpx.DecodeJSON(w, r, &entry); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// The path names the club, so a body claiming a different one must not
	// redirect the entry.
	entry.TeamID = r.PathValue("id")

	if err := h.service.Enter(r.Context(), &entry); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, entry)
}

// Withdraw removes an entry.
func (h *TeamSeasonHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Withdraw(r.Context(), r.PathValue("entryID")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
