package service

import (
	"context"

	"football-database-app/apps/football-svc/internal/domain"
	"football-database-app/apps/football-svc/internal/repository"
	"football-database-app/libs/auth"
)

type PlayerService interface {
	GetPlayer(ctx context.Context, id string) (*domain.Player, error)
	ListPlayersByTeam(ctx context.Context, teamID string) ([]domain.Player, error)
	ListPlayers(ctx context.Context, freeAgent *bool, position *string) ([]domain.Player, error) // New method
	CreatePlayer(ctx context.Context, player *domain.Player) error
	UpdatePlayer(ctx context.Context, player *domain.Player) error
	DeletePlayer(ctx context.Context, id string) error
}

type playerService struct {
	repo repository.PlayerRepository
}

func NewPlayerService(repo repository.PlayerRepository) PlayerService {
	return &playerService{repo: repo}
}

func (s *playerService) GetPlayer(ctx context.Context, id string) (*domain.Player, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *playerService) ListPlayersByTeam(ctx context.Context, teamID string) ([]domain.Player, error) {
	return s.repo.ListByTeam(ctx, teamID)
}

// ListPlayers implements the new method for filtering
func (s *playerService) ListPlayers(ctx context.Context, freeAgent *bool, position *string) ([]domain.Player, error) {
	// Public endpoint, no RBAC check for read access
	return s.repo.ListPlayers(ctx, freeAgent, position)
}

func (s *playerService) CreatePlayer(ctx context.Context, player *domain.Player) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}

	if claims.HasRole("admin") {
		// Admin: Full global CRUD access
	} else if claims.HasRole("editor") {
		// Editor: Allowed only if target team_id matches one of their managed team IDs
		if player.TeamID == nil || !claims.HasTeamPermission(*player.TeamID) {
			return ErrForbidden
		}
	} else {
		// User/Guest: Read-only access
		return ErrForbidden
	}
	return s.repo.Create(ctx, player)
}

func (s *playerService) UpdatePlayer(ctx context.Context, player *domain.Player) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}

	if claims.HasRole("admin") {
		// Admin: Full global CRUD access
	} else if claims.HasRole("editor") {
		// Editor: Requires permission for the player's current team (if any) and target team (if any, and different)
		existingPlayer, err := s.repo.GetByID(ctx, player.ID)
		if err != nil {
			return ErrNotFound // Player not found, cannot update
		}

		currentTeamID := existingPlayer.TeamID // Pointer to string
		targetTeamID := player.TeamID          // Pointer to string from the update request

		// Rule 1: Editor assigned to player's CURRENT team for contract updates
		if currentTeamID != nil {
			if !claims.HasTeamPermission(*currentTeamID) {
				return ErrForbidden // Editor does not manage the player's current team
			}
		}

		// Rule 2: Editor assigned to player's TARGET team to process transfers
		if targetTeamID != nil {
			// If it's a transfer to a new team or signing a free agent to a team
			if currentTeamID == nil || (currentTeamID != nil && *targetTeamID != *currentTeamID) {
				if !claims.HasTeamPermission(*targetTeamID) {
					return ErrForbidden // Editor does not manage the player's target team for transfer/assignment
				}
			}
		} else { // targetTeamID is nil (player becomes free agent or remains free agent)
			// If the player was already a free agent and remains a free agent.
			// Or if a player on a team becomes a free agent.
			// The rule states: "assigned to the player's current team (for contract updates) or target team (to process transfers)".
			// If `targetTeamID` is nil, it's either keeping a free agent or making a player a free agent.
			// If `currentTeamID` was not nil (player was on a team), then the first check `currentTeamID != nil` already validated permission to release them.
			// If `currentTeamID` was nil (player was already free agent) and `targetTeamID` is still nil (remains free agent),
			// an editor cannot modify free agents who remain free agents, as they don't manage any relevant team.
			if currentTeamID == nil {
				return ErrForbidden
			}
		}

	} else { // User/Guest: Read-only access
		return ErrForbidden
	}
	return s.repo.Update(ctx, player)
}

func (s *playerService) DeletePlayer(ctx context.Context, id string) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}

	if claims.HasRole("admin") {
		// Admin: Full global CRUD access
	} else if claims.HasRole("editor") {
		// Editor: Allowed only if the player belongs to one of their managed team IDs
		player, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return err // Or a specific not found error like ErrNotFound
		}
		if player.TeamID == nil || !claims.HasTeamPermission(*player.TeamID) {
			return ErrForbidden
		}
	} else {
		// User/Guest: Read-only access
		return ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}
