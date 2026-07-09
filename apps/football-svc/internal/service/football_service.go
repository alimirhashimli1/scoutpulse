package service

import (
	"context"

	"github.com/scoutpulse/libs/auth"
)

// FootballService owns cross-entity authorization rules for football writes.
type FootballService struct{}

func NewFootballService() FootballService {
	return FootballService{}
}

func (s FootballService) requireAdmin(ctx context.Context) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if !claims.HasRole("admin") {
		return ErrForbidden
	}
	return nil
}

func (s FootballService) requireAdminOrManagedTeam(ctx context.Context, teamID string) error {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if claims.HasRole("admin") {
		return nil
	}
	if claims.HasRole("editor") && claims.HasTeamPermission(teamID) {
		return nil
	}
	return ErrForbidden
}

func (s FootballService) requireAdminOrManagedTargetTeam(ctx context.Context, teamID *string) error {
	if teamID == nil {
		claims, ok := auth.GetClaims(ctx)
		if !ok {
			return ErrUnauthorized
		}
		if claims.HasRole("admin") {
			return nil
		}
		return ErrForbidden
	}
	return s.requireAdminOrManagedTeam(ctx, *teamID)
}

var footballAuthz = NewFootballService()
