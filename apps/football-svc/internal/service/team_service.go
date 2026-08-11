package service

import (
	"context"
	"time"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/events"
)

type TeamService interface {
	GetTeam(ctx context.Context, id string) (*domain.Team, error)
	ListTeamsByLeague(ctx context.Context, leagueID string, page domain.Page) ([]domain.Team, error)
	CreateTeam(ctx context.Context, team *domain.Team) error
	UpdateTeam(ctx context.Context, team *domain.Team) error
	DeleteTeam(ctx context.Context, id string) error
}

type teamService struct {
	repo      repository.TeamRepository
	authz     *Authorizer
	publisher events.Publisher
}

func NewTeamService(repo repository.TeamRepository, authz *Authorizer, publisher events.Publisher) TeamService {
	return &teamService{repo: repo, authz: authz, publisher: publisher}
}

func (s *teamService) GetTeam(ctx context.Context, id string) (*domain.Team, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *teamService) ListTeamsByLeague(ctx context.Context, leagueID string, page domain.Page) ([]domain.Team, error) {
	return s.repo.ListByLeague(ctx, leagueID, page)
}

func validateTeam(t *domain.Team) error {
	if t.Name == "" {
		return apperr.Invalid("name is required")
	}
	// A founding year in the future, or before the codification of the game,
	// is a data-entry error rather than a record worth storing.
	if t.FoundedYear != nil {
		year := *t.FoundedYear
		if year < 1850 || year > time.Now().Year() {
			return apperr.Invalid("founded_year must be between 1850 and the current year")
		}
	}
	return nil
}

// CreateTeam is admin-only: a club has no editor grants until it exists, so
// there is no grant that could authorize creating it.
func (s *teamService) CreateTeam(ctx context.Context, team *domain.Team) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	if err := validateTeam(team); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, team); err != nil {
		return err
	}

	_ = s.publisher.Publish(ctx, events.SubjectTeamCreated, events.TeamCreated{
		TeamID:   team.ID,
		Name:     team.Name,
		LeagueID: team.LeagueID,
	})
	return nil
}

// UpdateTeam edits a club's descriptive fields.
//
// A club's editor may not change its league. Which competition a club plays in
// is a claim about its standing, and letting the club's own editor set it means
// any editor can promote their club into any competition in the database -- the
// same reasoning that makes market values administrator-only, since otherwise
// every editor could inflate their own squad's valuations.
func (s *teamService) UpdateTeam(ctx context.Context, team *domain.Team) error {
	// Authorization first: an unauthenticated or unauthorized caller must not
	// cause a database read.
	if err := s.authz.RequireTeam(ctx, team.ID); err != nil {
		return err
	}
	if err := validateTeam(team); err != nil {
		return err
	}

	// Only an administrator may move a club between competitions. For anyone
	// else the stored value is preserved rather than the request rejected,
	// which is how UpdatePlayer treats team_id: a client that round-trips a
	// GET into a PUT should not fail merely for echoing a field back.
	if err := s.authz.RequireAdmin(ctx); err != nil {
		existing, err := s.repo.GetByID(ctx, team.ID)
		if err != nil {
			return err
		}
		team.LeagueID = existing.LeagueID
	}

	return s.repo.Update(ctx, team)
}

// DeleteTeam removes a club. Its players are not deleted with it: the schema's
// ON DELETE SET NULL turns them into free agents.
func (s *teamService) DeleteTeam(ctx context.Context, id string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// The club's editor grants cascade away with it, so drop the cached
	// answers too rather than leaving them to expire. The other two mutations
	// of grant state already do this.
	s.authz.InvalidateTeam(id)

	_ = s.publisher.Publish(ctx, events.SubjectTeamDeleted, events.TeamDeleted{TeamID: id})
	return nil
}
