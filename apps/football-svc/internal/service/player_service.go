package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
)

type PlayerService interface {
	GetPlayer(ctx context.Context, id string) (*domain.Player, error)
	ListPlayersByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.Player, error)
	ListPlayers(ctx context.Context, filter repository.PlayerFilter, page domain.Page) ([]domain.Player, error)
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

func (s *playerService) ListPlayersByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.Player, error) {
	return s.repo.List(ctx, repository.PlayerFilter{TeamID: &teamID}, page)
}

// ListPlayers is a public read; no authorization check applies.
func (s *playerService) ListPlayers(ctx context.Context, filter repository.PlayerFilter, page domain.Page) ([]domain.Player, error) {
	return s.repo.List(ctx, filter, page)
}

func (s *playerService) CreatePlayer(ctx context.Context, player *domain.Player) error {
	if err := footballAuthz.requireAdminOrManagedTargetTeam(ctx, player.TeamID); err != nil {
		return err
	}
	return s.repo.Create(ctx, player)
}

func (s *playerService) UpdatePlayer(ctx context.Context, player *domain.Player) error {
	// The existing row is loaded first so that a transfer can be authorized
	// against both the current squad and the destination squad: an editor who
	// manages either side may move the player.
	existing, err := s.repo.GetByID(ctx, player.ID)
	if err != nil {
		return err
	}

	if err := footballAuthz.requireAdminOrManagedCurrentOrTargetTeam(ctx, existing.TeamID, player.TeamID); err != nil {
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
