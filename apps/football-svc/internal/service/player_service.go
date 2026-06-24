package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
)

type PlayerService interface {
	GetPlayer(ctx context.Context, id string) (*domain.Player, error)
	ListPlayersByTeam(ctx context.Context, teamID string) ([]domain.Player, error)
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

func (s *playerService) CreatePlayer(ctx context.Context, player *domain.Player) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if claims.HasRole("admin") {
		// Admin: allowed to create globally
	} else if claims.HasRole("editor") {
		// Editor: allowed only if target team_id matches managed team IDs
		if player.TeamID == nil || !claims.HasTeamPermission(*player.TeamID) {
			return ErrForbidden
		}
	} else {
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
		// Admin: allowed to update globally
	} else if claims.HasRole("editor") {
		// Editor: allowed only if target team_id matches managed team IDs
		if player.TeamID == nil || !claims.HasTeamPermission(*player.TeamID) {
			return ErrForbidden
		}
	} else {
		return ErrForbidden
	}
	return s.repo.Update(ctx, player)
}

func (s *playerService) DeletePlayer(ctx context.Context, id string) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if !claims.HasRole("admin") {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}
