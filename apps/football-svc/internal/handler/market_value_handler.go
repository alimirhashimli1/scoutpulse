package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
)

type MarketValueHandler struct {
	service service.MarketValueService
}

func NewMarketValueHandler(s service.MarketValueService) *MarketValueHandler {
	return &MarketValueHandler{service: s}
}

// ListValues serves a player's valuation history, newest first.
func (h *MarketValueHandler) ListValues(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("id")
	if playerID == "" {
		httpx.WriteError(w, r, apperr.Invalid("player id is required"))
		return
	}

	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	values, err := h.service.ListValues(r.Context(), playerID, page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(values, page))
}

func (h *MarketValueHandler) RecordValue(w http.ResponseWriter, r *http.Request) {
	var value domain.MarketValue
	if err := httpx.DecodeJSON(w, r, &value); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// The path names the player, so a body claiming a different one must not
	// redirect the valuation.
	value.PlayerID = r.PathValue("id")

	if err := h.service.RecordValue(r.Context(), &value); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, value)
}

func (h *MarketValueHandler) DeleteValue(w http.ResponseWriter, r *http.Request) {
	// Both path segments are passed through: the valuation must belong to the
	// player the URL names.
	if err := h.service.DeleteValue(r.Context(), r.PathValue("id"), r.PathValue("valueID")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
