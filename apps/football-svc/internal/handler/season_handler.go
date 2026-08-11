package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/httpx"
)

type SeasonHandler struct {
	service service.SeasonService
}

func NewSeasonHandler(s service.SeasonService) *SeasonHandler {
	return &SeasonHandler{service: s}
}

func (h *SeasonHandler) ListSeasons(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	seasons, err := h.service.ListSeasons(r.Context(), page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(seasons, page))
}

// GetCurrentSeason is registered before the {id} route so "current" is not
// mistaken for an identifier.
func (h *SeasonHandler) GetCurrentSeason(w http.ResponseWriter, r *http.Request) {
	season, err := h.service.CurrentSeason(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, season)
}

func (h *SeasonHandler) GetSeason(w http.ResponseWriter, r *http.Request) {
	season, err := h.service.GetSeason(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, season)
}

func (h *SeasonHandler) CreateSeason(w http.ResponseWriter, r *http.Request) {
	var season domain.Season
	if err := httpx.DecodeJSON(w, r, &season); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err := h.service.CreateSeason(r.Context(), &season); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, season)
}

func (h *SeasonHandler) UpdateSeason(w http.ResponseWriter, r *http.Request) {
	var season domain.Season
	if err := httpx.DecodeJSON(w, r, &season); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	season.ID = r.PathValue("id")

	if err := h.service.UpdateSeason(r.Context(), &season); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, season)
}

func (h *SeasonHandler) DeleteSeason(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteSeason(r.Context(), r.PathValue("id")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
