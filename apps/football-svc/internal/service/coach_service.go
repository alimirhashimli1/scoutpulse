package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
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
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if claims.HasRole("admin") {
		// Admin: allowed to create globally
	} else if claims.HasRole("editor") {
		// Editor: allowed only if target team_id matches managed team IDs
		if coach.TeamID == nil || !claims.HasTeamPermission(*coach.TeamID) {
			return ErrForbidden
		}
	} else {
		return ErrForbidden
	}
	return s.repo.Create(ctx, coach)
}

func (s *coachService) UpdateCoach(ctx context.Context, coach *domain.Coach) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if claims.HasRole("admin") {
		// Admin: allowed to update globally
	} else if claims.HasRole("editor") {
		// Editor: allowed only if target team_id matches managed team IDs
		if coach.TeamID == nil || !claims.HasTeamPermission(*coach.TeamID) {
			return ErrForbidden
		}
	} else {
		return ErrForbidden
	}
	return s.repo.Update(ctx, coach)
}

func (s *coachService) DeleteCoach(ctx context.Context, id string) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if !claims.HasRole("admin") {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}
