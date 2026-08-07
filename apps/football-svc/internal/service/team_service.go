package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
)

type TeamService interface {
	GetTeam(ctx context.Context, id string) (*domain.Team, error)
	ListTeamsByLeague(ctx context.Context, leagueID string, page domain.Page) ([]domain.Team, error)
	CreateTeam(ctx context.Context, team *domain.Team) error
	UpdateTeam(ctx context.Context, team *domain.Team) error
	DeleteTeam(ctx context.Context, id string) error
}

type teamService struct {
	repo repository.TeamRepository
}

func NewTeamService(repo repository.TeamRepository) TeamService {
	return &teamService{repo: repo}
}

func (s *teamService) GetTeam(ctx context.Context, id string) (*domain.Team, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *teamService) ListTeamsByLeague(ctx context.Context, leagueID string, page domain.Page) ([]domain.Team, error) {
	return s.repo.ListByLeague(ctx, leagueID, page)
}

func (s *teamService) CreateTeam(ctx context.Context, team *domain.Team) error {
	if err := footballAuthz.requireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Create(ctx, team)
}

func (s *teamService) UpdateTeam(ctx context.Context, team *domain.Team) error {
	if err := footballAuthz.requireAdminOrManagedTeam(ctx, team.ID); err != nil {
		return err
	}
	return s.repo.Update(ctx, team)
}

func (s *teamService) DeleteTeam(ctx context.Context, id string) error {
	if err := footballAuthz.requireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
