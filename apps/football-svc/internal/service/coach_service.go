package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
)

type CoachService interface {
	GetCoach(ctx context.Context, id string) (*domain.Coach, error)
	GetCoachByTeam(ctx context.Context, teamID string) (*domain.Coach, error)
	CreateCoach(ctx context.Context, coach *domain.Coach) error
	UpdateCoach(ctx context.Context, coach *domain.Coach) error
	DeleteCoach(ctx context.Context, id string) error
}

type coachService struct {
	repo  repository.CoachRepository
	authz *Authorizer
}

func NewCoachService(repo repository.CoachRepository, authz *Authorizer) CoachService {
	return &coachService{repo: repo, authz: authz}
}

func (s *coachService) GetCoach(ctx context.Context, id string) (*domain.Coach, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *coachService) GetCoachByTeam(ctx context.Context, teamID string) (*domain.Coach, error) {
	return s.repo.GetByTeam(ctx, teamID)
}

func validateCoach(c *domain.Coach) error {
	if c.Name == "" {
		return apperr.Invalid("name is required")
	}
	return nil
}

func (s *coachService) CreateCoach(ctx context.Context, coach *domain.Coach) error {
	if err := s.authz.RequireTargetTeam(ctx, coach.TeamID); err != nil {
		return err
	}
	if err := validateCoach(coach); err != nil {
		return err
	}
	return s.repo.Create(ctx, coach)
}

// UpdateCoach edits descriptive fields only. Changing which club a coach is at
// is an appointment, recorded through CoachSpellService.
func (s *coachService) UpdateCoach(ctx context.Context, coach *domain.Coach) error {
	existing, err := s.repo.GetByID(ctx, coach.ID)
	if err != nil {
		return err
	}

	if err := s.authz.RequireTargetTeam(ctx, existing.TeamID); err != nil {
		return err
	}
	if err := validateCoach(coach); err != nil {
		return err
	}

	coach.TeamID = existing.TeamID
	return s.repo.Update(ctx, coach)
}

func (s *coachService) DeleteCoach(ctx context.Context, id string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
