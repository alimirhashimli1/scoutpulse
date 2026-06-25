package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
)

type TeamService interface {
	GetTeam(ctx context.Context, id string) (*domain.Team, error)
	ListTeamsByLeague(ctx context.Context, leagueID string) ([]domain.Team, error)
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

func (s *teamService) ListTeamsByLeague(ctx context.Context, leagueID string) ([]domain.Team, error) {
	return s.repo.ListByLeague(ctx, leagueID)
}

func (s *teamService) CreateTeam(ctx context.Context, team *domain.Team) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if !claims.HasRole("admin") {
		return ErrForbidden
	}
	return s.repo.Create(ctx, team)
}

func (s *teamService) UpdateTeam(ctx context.Context, team *domain.Team) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if !claims.HasTeamPermission(team.ID) {
		return ErrForbidden
	}
	return s.repo.Update(ctx, team)
}

func (s *teamService) DeleteTeam(ctx context.Context, id string) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if !claims.HasRole("admin") {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}
