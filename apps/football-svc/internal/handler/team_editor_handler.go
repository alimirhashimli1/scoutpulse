package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
)

// TeamEditorHandler administers which users may edit which clubs.
type TeamEditorHandler struct {
	service service.TeamEditorService
}

func NewTeamEditorHandler(s service.TeamEditorService) *TeamEditorHandler {
	return &TeamEditorHandler{service: s}
}

type grantRequest struct {
	UserID string `json:"user_id"`
}

func (h *TeamEditorHandler) ListEditors(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	editors, err := h.service.ListEditors(r.Context(), r.PathValue("id"), page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(editors, page))
}

// ListMyTeams reports the clubs the caller may edit. This is what replaced
// reading managed_team_ids out of the token.
func (h *TeamEditorHandler) ListMyTeams(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")

	teams, err := h.service.ListTeamsForUser(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if teams == nil {
		teams = []string{}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user_id": userID, "team_ids": teams})
}

func (h *TeamEditorHandler) Grant(w http.ResponseWriter, r *http.Request) {
	var req grantRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if req.UserID == "" {
		httpx.WriteError(w, r, apperr.Invalid("user_id is required"))
		return
	}

	teamID := r.PathValue("id")
	if err := h.service.Grant(r.Context(), req.UserID, teamID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, repository.TeamEditor{UserID: req.UserID, TeamID: teamID})
}

func (h *TeamEditorHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Revoke(r.Context(), r.PathValue("userID"), r.PathValue("id")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
