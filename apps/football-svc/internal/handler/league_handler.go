package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
)

type LeagueHandler struct {
	service service.LeagueService
}

func NewLeagueHandler(service service.LeagueService) *LeagueHandler {
	return &LeagueHandler{service: service}
}

func (h *LeagueHandler) ListLeagues(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	leagues, err := h.service.ListLeagues(r.Context(), page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(leagues, page))
}

func (h *LeagueHandler) GetLeague(w http.ResponseWriter, r *http.Request) {
	league, err := h.service.GetLeague(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, league)
}

func (h *LeagueHandler) CreateLeague(w http.ResponseWriter, r *http.Request) {
	var league domain.League
	if err := httpx.DecodeJSON(w, r, &league); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if league.Name == "" || league.Country == "" {
		httpx.WriteError(w, r, apperr.Invalid("name and country are required"))
		return
	}

	if err := h.service.CreateLeague(r.Context(), &league); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, league)
}

func (h *LeagueHandler) UpdateLeague(w http.ResponseWriter, r *http.Request) {
	var league domain.League
	if err := httpx.DecodeJSON(w, r, &league); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	league.ID = r.PathValue("id")

	if err := h.service.UpdateLeague(r.Context(), &league); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, league)
}

func (h *LeagueHandler) DeleteLeague(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteLeague(r.Context(), r.PathValue("id")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
