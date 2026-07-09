package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
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
	if err := footballAuthz.requireAdminOrManagedTargetTeam(ctx, player.TeamID); err != nil {
		return err
	}
	return s.repo.Create(ctx, player)
}

func (s *playerService) UpdatePlayer(ctx context.Context, player *domain.Player) error {
	if err := footballAuthz.requireAdminOrManagedTargetTeam(ctx, player.TeamID); err != nil {
		return err
	}
	return s.repo.Update(ctx, player)
}

func (s *playerService) DeletePlayer(ctx context.Context, id string) error {
	if err := footballAuthz.requireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
