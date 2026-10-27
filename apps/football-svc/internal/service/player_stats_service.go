package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
)

// PlayerStatsService serves season-by-season performance. Read-only: the rows
// are written by the importer, never through the API.
type PlayerStatsService interface {
	ListStats(ctx context.Context, playerID string) ([]domain.PlayerSeasonStat, error)
}

type playerStatsService struct {
	repo repository.PlayerStatsRepository
}

func NewPlayerStatsService(repo repository.PlayerStatsRepository) PlayerStatsService {
	return &playerStatsService{repo: repo}
}

// ListStats is a public read, like the rest of a player's profile.
func (s *playerStatsService) ListStats(ctx context.Context, playerID string) ([]domain.PlayerSeasonStat, error) {
	return s.repo.ListByPlayer(ctx, playerID)
}
