package handler

import (
	"encoding/json"
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/auth"
)

type CoachHandler struct {
	service service.CoachService
}

func NewCoachHandler(service service.CoachService) *CoachHandler {
	return &CoachHandler{service: service}
}

func (h *CoachHandler) GetCoachByTeam(w http.ResponseWriter, r *http.Request) {
	teamID := r.URL.Query().Get("team_id")
	if teamID == "" {
		http.Error(w, "team_id is required", http.StatusBadRequest)
		return
	}

	coach, err := h.service.GetCoachByTeam(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(coach)
}

func (h *CoachHandler) CreateCoach(w http.ResponseWriter, r *http.Request) {
	var coach domain.Coach
	if err := json.NewDecoder(r.Body).Decode(&coach); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		if coach.TeamID == nil || !claims.HasTeamPermission(*coach.TeamID) {
			http.Error(w, "Forbidden: You do not manage this team", http.StatusForbidden)
			return
		}
	} else {
		// User/Guest: Reject
		http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
		return
	}

	if err := h.service.CreateCoach(r.Context(), &coach); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(coach)
}
