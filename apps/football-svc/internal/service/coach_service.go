package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
)

type CoachService interface {
	GetCoach(ctx context.Context, id string) (*domain.Coach, error)
	GetCoachByTeam(ctx context.Context, teamID string) (*domain.Coach, error)
	CreateCoach(ctx context.Context, coach *domain.Coach) error
	UpdateCoach(ctx context.Context, coach *domain.Coach) error
	DeleteCoach(ctx context.Context, id string) error
}

type coachService struct {
	repo repository.CoachRepository
}

func NewCoachService(repo repository.CoachRepository) CoachService {
	return &coachService{repo: repo}
}

func (s *coachService) GetCoach(ctx context.Context, id string) (*domain.Coach, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *coachService) GetCoachByTeam(ctx context.Context, teamID string) (*domain.Coach, error) {
	return s.repo.GetByTeam(ctx, teamID)
}

func (s *coachService) CreateCoach(ctx context.Context, coach *domain.Coach) error {
	return s.repo.Create(ctx, coach)
}

func (s *coachService) UpdateCoach(ctx context.Context, coach *domain.Coach) error {
	return s.repo.Update(ctx, coach)
}

func (s *coachService) DeleteCoach(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
