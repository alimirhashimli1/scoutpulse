package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
)

type PlayerStatsHandler struct {
	service service.PlayerStatsService
}

func NewPlayerStatsHandler(s service.PlayerStatsService) *PlayerStatsHandler {
	return &PlayerStatsHandler{service: s}
}

// ListStats serves a player's season lines, newest first.
//
// The whole career in one response, in the same list envelope as the paged
// endpoints so clients read it the same way; has_more is always false.
func (h *PlayerStatsHandler) ListStats(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("id")
	if playerID == "" {
		httpx.WriteError(w, r, apperr.Invalid("player id is required"))
		return
	}

	stats, err := h.service.ListStats(r.Context(), playerID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.ListResult[domain.PlayerSeasonStat]{
		Items: stats, Limit: len(stats), Offset: 0, HasMore: false,
	})
}
