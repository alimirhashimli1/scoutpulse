package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
)

type PlayerHandler struct {
	service service.PlayerService
}

func NewPlayerHandler(service service.PlayerService) *PlayerHandler {
	return &PlayerHandler{service: service}
}

func (h *PlayerHandler) ListPlayers(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	freeAgent, err := httpx.QueryBool(r, "free_agent")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	filter := repository.PlayerFilter{
		FreeAgent:   freeAgent,
		Position:    httpx.QueryString(r, "position"),
		TeamID:      httpx.QueryString(r, "team_id"),
		Nationality: httpx.QueryString(r, "nationality"),
	}

	// Value bounds are in minor units, matching the wire format for money.
	for _, bound := range []struct {
		name string
		dst  **domain.Minor
	}{
		{"min_value_minor", &filter.MinValue},
		{"max_value_minor", &filter.MaxValue},
	} {
		if r.URL.Query().Get(bound.name) == "" {
			continue
		}
		raw, err := httpx.QueryInt(r, bound.name, 0)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		v := domain.Minor(raw)
		*bound.dst = &v
	}

	players, err := h.service.ListPlayers(r.Context(), filter, page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(players, page))
}

func (h *PlayerHandler) GetPlayer(w http.ResponseWriter, r *http.Request) {
	player, err := h.service.GetPlayer(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, player)
}

func (h *PlayerHandler) CreatePlayer(w http.ResponseWriter, r *http.Request) {
	var player domain.Player
	if err := httpx.DecodeJSON(w, r, &player); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if player.Name == "" {
		httpx.WriteError(w, r, apperr.Invalid("name is required"))
		return
	}

	if err := h.service.CreatePlayer(r.Context(), &player); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, player)
}

func (h *PlayerHandler) UpdatePlayer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		httpx.WriteError(w, r, apperr.Invalid("id is required"))
		return
	}

	var player domain.Player
	if err := httpx.DecodeJSON(w, r, &player); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// The path is authoritative: a body that names a different id must not be
	// able to redirect the update to another row.
	player.ID = id

	if err := h.service.UpdatePlayer(r.Context(), &player); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, player)
}

func (h *PlayerHandler) DeletePlayer(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeletePlayer(r.Context(), r.PathValue("id")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
