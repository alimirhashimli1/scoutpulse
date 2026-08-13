package handler

import (
	"errors"
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
)

// squadPageSize bounds the squad embedded in a team detail response. A squad
// is small enough to return whole, but the bound keeps the response size
// predictable regardless of the data.
const squadPageSize = domain.MaxPageSize

type TeamHandler struct {
	teamService   service.TeamService
	playerService service.PlayerService
	coachService  service.CoachService
}

func NewTeamHandler(teamService service.TeamService, playerService service.PlayerService, coachService service.CoachService) *TeamHandler {
	return &TeamHandler{
		teamService:   teamService,
		playerService: playerService,
		coachService:  coachService,
	}
}

// ListTeams serves clubs, optionally narrowed to one competition.
//
// league_id used to be mandatory, which meant there was no way to list every
// club -- a browse-all page could not be built at all. It is now an ordinary
// optional filter, like the ones on players and transfers.
func (h *TeamHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	teams, err := h.teamService.ListTeams(r.Context(), httpx.QueryString(r, "league_id"), page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(teams, page))
}

// TeamDetailResponse embeds the squad and head coach alongside the team.
type TeamDetailResponse struct {
	domain.Team
	Players []domain.Player `json:"players"`
	Coach   *domain.Coach   `json:"coach,omitempty"`
}

func (h *TeamHandler) GetTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		httpx.WriteError(w, r, apperr.Invalid("id is required"))
		return
	}

	team, err := h.teamService.GetTeam(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	squadPage := domain.NewPage(squadPageSize, 0)
	players, err := h.playerService.ListPlayersByTeam(r.Context(), id, squadPage)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(players) > squadPage.Limit {
		players = players[:squadPage.Limit]
	}

	// A team without a head coach is normal, so a missing coach is not an
	// error; any other failure is.
	coach, err := h.coachService.GetCoachByTeam(r.Context(), id)
	if err != nil && apperr.KindOf(err) != apperr.KindNotFound && !errors.Is(err, service.ErrNotFound) {
		httpx.WriteError(w, r, err)
		return
	}
	if err != nil {
		coach = nil
	}

	httpx.WriteJSON(w, http.StatusOK, TeamDetailResponse{
		Team:    *team,
		Players: players,
		Coach:   coach,
	})
}

func (h *TeamHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var team domain.Team
	if err := httpx.DecodeJSON(w, r, &team); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if team.Name == "" {
		httpx.WriteError(w, r, apperr.Invalid("name is required"))
		return
	}

	if err := h.teamService.CreateTeam(r.Context(), &team); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, team)
}

func (h *TeamHandler) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		httpx.WriteError(w, r, apperr.Invalid("id is required"))
		return
	}

	var team domain.Team
	if err := httpx.DecodeJSON(w, r, &team); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// The service authorizes against this id, so it must come from the path
	// and not from a body the caller controls.
	team.ID = id

	if err := h.teamService.UpdateTeam(r.Context(), &team); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, team)
}

func (h *TeamHandler) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	if err := h.teamService.DeleteTeam(r.Context(), r.PathValue("id")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
