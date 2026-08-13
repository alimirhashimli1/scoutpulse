package service

import (
	"context"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
)

// TeamSeasonService records which competitions a club contested in a season.
type TeamSeasonService interface {
	ListByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.TeamSeason, error)
	ListBySeason(ctx context.Context, seasonID string, leagueID *string, page domain.Page) ([]domain.TeamSeason, error)
	Enter(ctx context.Context, entry *domain.TeamSeason) error
	Withdraw(ctx context.Context, id string) error
}

type teamSeasonService struct {
	repo    repository.TeamSeasonRepository
	teams   repository.TeamRepository
	seasons repository.SeasonRepository
	leagues repository.LeagueRepository
	authz   *Authorizer
}

func NewTeamSeasonService(
	repo repository.TeamSeasonRepository,
	teams repository.TeamRepository,
	seasons repository.SeasonRepository,
	leagues repository.LeagueRepository,
	authz *Authorizer,
) TeamSeasonService {
	return &teamSeasonService{repo: repo, teams: teams, seasons: seasons, leagues: leagues, authz: authz}
}

// ListByTeam is a public read: a club's competition history is product data.
func (s *teamSeasonService) ListByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.TeamSeason, error) {
	if teamID == "" {
		return nil, apperr.Invalid("team id is required")
	}
	return s.repo.ListByTeam(ctx, teamID, page)
}

func (s *teamSeasonService) ListBySeason(ctx context.Context, seasonID string, leagueID *string, page domain.Page) ([]domain.TeamSeason, error) {
	if seasonID == "" {
		return nil, apperr.Invalid("season id is required")
	}
	return s.repo.ListBySeason(ctx, seasonID, leagueID, page)
}

// Enter records a club in a competition for a season.
//
// The club's own editor may do this: entering a competition is a statement
// about the club, and the editor already controls the club's current league
// only through an administrator. Creating the entry is checked against the
// club, not the competition, because a competition has no editors.
func (s *teamSeasonService) Enter(ctx context.Context, e *domain.TeamSeason) error {
	if e.TeamID == "" || e.SeasonID == "" || e.LeagueID == "" {
		return apperr.Invalid("team_id, season_id and league_id are required")
	}

	if err := s.authz.RequireTeam(ctx, e.TeamID); err != nil {
		return err
	}

	// Resolve each reference so a bad id is a clean 404 naming what is
	// missing, rather than one opaque foreign-key violation for three
	// possible causes.
	if _, err := s.teams.GetByID(ctx, e.TeamID); err != nil {
		return err
	}
	if _, err := s.seasons.GetByID(ctx, e.SeasonID); err != nil {
		return err
	}
	if _, err := s.leagues.GetByID(ctx, e.LeagueID); err != nil {
		return err
	}

	return s.repo.Enter(ctx, e)
}

// Withdraw removes an entry. Admin-only: by the time a season is under way,
// removing a club from a competition it played in is rewriting history rather
// than correcting a club's own record.
func (s *teamSeasonService) Withdraw(ctx context.Context, id string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Withdraw(ctx, id)
}
