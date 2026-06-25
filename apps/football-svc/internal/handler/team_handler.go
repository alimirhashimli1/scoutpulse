package handler

import (
	"encoding/json"
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/auth"
)

type TeamHandler struct {
	teamService  service.TeamService
	playerService service.PlayerService
	coachService service.CoachService
}

func NewTeamHandler(teamService service.TeamService, playerService service.PlayerService, coachService service.CoachService) *TeamHandler {
	return &TeamHandler{
		teamService:  teamService,
		playerService: playerService,
		coachService: coachService,
	}
}

func (h *TeamHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	leagueID := r.URL.Query().Get("league_id")
	if leagueID == "" {
		http.Error(w, "league_id is required", http.StatusBadRequest)
		return
	}

	teams, err := h.service.ListTeamsByLeague(r.Context(), leagueID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(teams)
}

// TeamDetailResponse is the response structure for GetTeam that includes nested players and coach.
type TeamDetailResponse struct {
	domain.Team
	Players []domain.Player `json:"players"`
	Coach   *domain.Coach   `json:"coach,omitempty"`
}

func (h *TeamHandler) GetTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	team, err := h.teamService.GetTeam(r.Context(), id)
	if err != nil {
		if err == service.ErrNotFound { // Assuming service layer returns a specific error for not found
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	players, err := h.playerService.ListPlayersByTeam(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	coach, err := h.coachService.GetCoachByTeam(r.Context(), id)
	if err != nil && err != service.ErrNotFound { // Coach might not exist, but other errors should be handled
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// If coach is not found, coach will be nil, which is handled by omitempty in JSON tag

	response := TeamDetailResponse{
		Team:    *team,
		Players: players,
		Coach:   coach,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *TeamHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	// RBAC logic
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if claims.Role != "admin" {
		http.Error(w, "Forbidden: Admin access only", http.StatusForbidden)
		return
	}

	var team domain.Team
	if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.CreateTeam(r.Context(), &team); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(team)
}

func (h *TeamHandler) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	// RBAC logic
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if claims.Role == "admin" {
		// Admin: Grant access immediately
	} else if claims.Role == "editor" {
		// Editor: Check if manages this team
		if !claims.HasTeamPermission(id) {
			http.Error(w, "Forbidden: You do not manage this team", http.StatusForbidden)
			return
		}
	} else {
		// User/Guest: Reject
		http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
		return
	}

	var team domain.Team
	if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	team.ID = id

	if err := h.service.UpdateTeam(r.Context(), &team); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(team)
}
