package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
)

type CoachHandler struct {
	service service.CoachService
}

func NewCoachHandler(service service.CoachService) *CoachHandler {
	return &CoachHandler{service: service}
}

// GetCoach serves one coach.
//
// Until this existed a coach was only reachable through their club or their
// spells, so a coach profile page had no way to load its subject.
func (h *CoachHandler) GetCoach(w http.ResponseWriter, r *http.Request) {
	coach, err := h.service.GetCoach(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, coach)
}

func (h *CoachHandler) GetCoachByTeam(w http.ResponseWriter, r *http.Request) {
	teamID := r.URL.Query().Get("team_id")
	if teamID == "" {
		httpx.WriteError(w, r, apperr.Invalid("team_id is required"))
		return
	}

	coach, err := h.service.GetCoachByTeam(r.Context(), teamID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, coach)
}

func (h *CoachHandler) CreateCoach(w http.ResponseWriter, r *http.Request) {
	var coach domain.Coach
	if err := httpx.DecodeJSON(w, r, &coach); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if coach.Name == "" {
		httpx.WriteError(w, r, apperr.Invalid("name is required"))
		return
	}

	if err := h.service.CreateCoach(r.Context(), &coach); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, coach)
}

func (h *CoachHandler) UpdateCoach(w http.ResponseWriter, r *http.Request) {
	var coach domain.Coach
	if err := httpx.DecodeJSON(w, r, &coach); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	coach.ID = r.PathValue("id")

	if err := h.service.UpdateCoach(r.Context(), &coach); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, coach)
}

func (h *CoachHandler) DeleteCoach(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteCoach(r.Context(), r.PathValue("id")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
