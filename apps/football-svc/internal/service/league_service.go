package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
)

type LeagueService interface {
	GetLeague(ctx context.Context, id string) (*domain.League, error)
	ListLeagues(ctx context.Context) ([]domain.League, error)
	CreateLeague(ctx context.Context, league *domain.League) error
	UpdateLeague(ctx context.Context, league *domain.League) error
	DeleteLeague(ctx context.Context, id string) error
}

type leagueService struct {
	repo repository.LeagueRepository
}

func NewLeagueService(repo repository.LeagueRepository) LeagueService {
	return &leagueService{repo: repo}
}

func (s *leagueService) GetLeague(ctx context.Context, id string) (*domain.League, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *leagueService) ListLeagues(ctx context.Context) ([]domain.League, error) {
	return s.repo.List(ctx)
}

func (s *leagueService) CreateLeague(ctx context.Context, league *domain.League) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}

	if !claims.HasRole("admin") {
		// Only Admin can create leagues
		return ErrForbidden
	}
	return s.repo.Create(ctx, league)
}

func (s *leagueService) UpdateLeague(ctx context.Context, league *domain.League) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}

	if !claims.HasRole("admin") {
		// Only Admin can update leagues
		return ErrForbidden
	}
	return s.repo.Update(ctx, league)
}

func (s *leagueService) DeleteLeague(ctx context.Context, id string) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}

	if !claims.HasRole("admin") {
		// Only Admin can delete leagues
		return ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}
