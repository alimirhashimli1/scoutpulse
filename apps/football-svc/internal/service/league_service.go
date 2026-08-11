package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
)

type LeagueService interface {
	GetLeague(ctx context.Context, id string) (*domain.League, error)
	ListLeagues(ctx context.Context, page domain.Page) ([]domain.League, error)
	CreateLeague(ctx context.Context, league *domain.League) error
	UpdateLeague(ctx context.Context, league *domain.League) error
	DeleteLeague(ctx context.Context, id string) error
}

type leagueService struct {
	repo  repository.LeagueRepository
	authz *Authorizer
}

func NewLeagueService(repo repository.LeagueRepository, authz *Authorizer) LeagueService {
	return &leagueService{repo: repo, authz: authz}
}

func (s *leagueService) GetLeague(ctx context.Context, id string) (*domain.League, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *leagueService) ListLeagues(ctx context.Context, page domain.Page) ([]domain.League, error) {
	return s.repo.List(ctx, page)
}

// validateLeague applies the invariants the database also enforces, so a bad
// request is a 400 with a useful message rather than a constraint violation.
func validateLeague(l *domain.League) error {
	if l.Name == "" || l.Country == "" {
		return apperr.Invalid("name and country are required")
	}
	if l.CompetitionType == "" {
		l.CompetitionType = domain.CompetitionLeague
	}
	if !domain.ValidCompetitionType(l.CompetitionType) {
		return apperr.Invalid("competition_type must be one of: league, domestic_cup, international_cup, super_cup")
	}
	if l.Tier != nil && *l.Tier < 1 {
		return apperr.Invalid("tier must be 1 or greater")
	}
	return nil
}

func (s *leagueService) CreateLeague(ctx context.Context, league *domain.League) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	if err := validateLeague(league); err != nil {
		return err
	}
	return s.repo.Create(ctx, league)
}

func (s *leagueService) UpdateLeague(ctx context.Context, league *domain.League) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	if err := validateLeague(league); err != nil {
		return err
	}
	return s.repo.Update(ctx, league)
}

func (s *leagueService) DeleteLeague(ctx context.Context, id string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
