package handler

import (
	"encoding/json"
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/auth"
)

type LeagueHandler struct {
	service service.LeagueService
}

func NewLeagueHandler(service service.LeagueService) *LeagueHandler {
	return &LeagueHandler{service: service}
}

func (h *LeagueHandler) ListLeagues(w http.ResponseWriter, r *http.Request) {
	leagues, err := h.service.ListLeagues(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(leagues)
}

func (h *LeagueHandler) CreateLeague(w http.ResponseWriter, r *http.Request) {
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

	var league domain.League
	if err := json.NewDecoder(r.Body).Decode(&league); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.CreateLeague(r.Context(), &league); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(league)
}
