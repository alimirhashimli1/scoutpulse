package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"football-database-app/apps/football-svc/internal/domain"
	"football-database-app/apps/football-svc/internal/service"
	"football-database-app/libs/auth"
)

type PlayerHandler struct {
	service service.PlayerService
}

func NewPlayerHandler(service service.PlayerService) *PlayerHandler {
	return &PlayerHandler{service: service}
}

func (h *PlayerHandler) ListPlayers(w http.ResponseWriter, r *http.Request) {
	var freeAgentFilter *bool
	if faStr := r.URL.Query().Get("free_agent"); faStr != "" {
		fa, err := strconv.ParseBool(faStr)
		if err != nil {
			http.Error(w, "Invalid value for free_agent", http.StatusBadRequest)
			return
		}
		freeAgentFilter = &fa
	}

	var positionFilter *string
	if posStr := r.URL.Query().Get("position"); posStr != "" {
		positionFilter = &posStr
	}

	players, err := h.service.ListPlayers(r.Context(), freeAgentFilter, positionFilter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(players)
}

func (h *PlayerHandler) CreatePlayer(w http.ResponseWriter, r *http.Request) {
	var player domain.Player
	if err := json.NewDecoder(r.Body).Decode(&player); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// RBAC logic - additional check for immediate feedback, service layer will also enforce
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if claims.HasRole("admin") {
		// Admin: allowed to create globally
	} else if claims.HasRole("editor") {
		// Editor: allowed only if target team_id matches managed team IDs
		if player.TeamID == nil || !claims.HasTeamPermission(*player.TeamID) {
			http.Error(w, "Forbidden: You do not manage the target team", http.StatusForbidden)
			return
		}
	} else {
		// User/Guest: Read-only access
		http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
		return
	}

	if err := h.service.CreatePlayer(r.Context(), &player); err != nil {
		// Service layer RBAC will return ErrForbidden, ErrUnauthorized etc.
		if err == service.ErrForbidden {
			http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
			return
		} else if err == service.ErrUnauthorized {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(player)
}

func (h *PlayerHandler) UpdatePlayer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	var player domain.Player
	if err := json.NewDecoder(r.Body).Decode(&player); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	player.ID = id // Ensure the ID from the URL path is used

	// RBAC logic is handled primarily in the service layer for comprehensive checks
	// The service layer will fetch the existing player to validate permissions against current and target teams.
	if err := h.service.UpdatePlayer(r.Context(), &player); err != nil {
		if err == service.ErrForbidden {
			http.Error(w, "Forbidden: Insufficient permissions for player update/transfer", http.StatusForbidden)
			return
		} else if err == service.ErrUnauthorized {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		} else if err == service.ErrNotFound {
			http.Error(w, "Player not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(player)
}
