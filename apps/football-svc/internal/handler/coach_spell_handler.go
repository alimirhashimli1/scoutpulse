package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/httpx"
)

type CoachSpellHandler struct {
	service service.CoachSpellService
}

func NewCoachSpellHandler(s service.CoachSpellService) *CoachSpellHandler {
	return &CoachSpellHandler{service: s}
}

// ListByCoach serves a coach's career history, newest first.
func (h *CoachSpellHandler) ListByCoach(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	spells, err := h.service.ListByCoach(r.Context(), r.PathValue("id"), page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(spells, page))
}

// ListByTeam serves a club's managerial history.
func (h *CoachSpellHandler) ListByTeam(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	spells, err := h.service.ListByTeam(r.Context(), r.PathValue("id"), page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(spells, page))
}

func (h *CoachSpellHandler) RecordSpell(w http.ResponseWriter, r *http.Request) {
	var spell domain.CoachSpell
	if err := httpx.DecodeJSON(w, r, &spell); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	spell.CoachID = r.PathValue("id")

	if err := h.service.RecordSpell(r.Context(), &spell); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, spell)
}

func (h *CoachSpellHandler) DeleteSpell(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteSpell(r.Context(), r.PathValue("spellID")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
