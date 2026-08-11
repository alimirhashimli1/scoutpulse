package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
)

// TeamEditorService administers which users may edit which clubs.
//
// These grants used to travel inside the JWT, where a revocation could not
// take effect until the token expired. They now live in the database and are
// resolved per request, so granting and revoking are both immediate.
type TeamEditorService interface {
	ListTeamsForUser(ctx context.Context, userID string) ([]string, error)
	ListEditors(ctx context.Context, teamID string, page domain.Page) ([]repository.TeamEditor, error)
	Grant(ctx context.Context, userID, teamID string) error
	Revoke(ctx context.Context, userID, teamID string) error
}

type teamEditorService struct {
	repo  repository.TeamEditorRepository
	teams repository.TeamRepository
	authz *Authorizer
}

func NewTeamEditorService(
	repo repository.TeamEditorRepository,
	teams repository.TeamRepository,
	authz *Authorizer,
) TeamEditorService {
	return &teamEditorService{repo: repo, teams: teams, authz: authz}
}

// ListTeamsForUser lets a caller read their own grants; only an administrator
// may read someone else's.
func (s *teamEditorService) ListTeamsForUser(ctx context.Context, userID string) ([]string, error) {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if claims.UserID != userID && claims.Role != RoleAdmin {
		return nil, ErrForbidden
	}
	return s.repo.ListTeams(ctx, userID)
}

func (s *teamEditorService) ListEditors(ctx context.Context, teamID string, page domain.Page) ([]repository.TeamEditor, error) {
	// Who may edit a club is access-control data, not public information.
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return s.repo.ListEditors(ctx, teamID, page)
}

// Grant is admin-only: an editor who could grant access to their own club
// could hand it to anyone, which makes the grant boundary meaningless.
func (s *teamEditorService) Grant(ctx context.Context, userID, teamID string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	if userID == "" || teamID == "" {
		return apperr.Invalid("user_id and team_id are required")
	}

	// There is no foreign key to validate against: users live in the identity
	// service's database, and this one cannot express the constraint. A format
	// check is what remains enforceable locally, and it catches the realistic
	// mistake -- a mistyped or truncated id silently written as a grant that
	// matches nobody and is invisible thereafter.
	//
	// It does not establish that the account exists. Verifying that means
	// calling identity-svc, and cleaning up after a deleted account means
	// subscribing to a user-deleted event; both are tracked in ISSUES.md as
	// the remaining half of N16.
	if _, err := uuid.Parse(userID); err != nil {
		return apperr.Invalid("user_id must be a UUID")
	}

	// Fail with a clear 404 rather than a foreign-key violation.
	if _, err := s.teams.GetByID(ctx, teamID); err != nil {
		return err
	}

	claims, _ := auth.GetClaims(ctx)
	var grantedBy *string
	if claims != nil && claims.UserID != "" {
		grantedBy = &claims.UserID
	}

	if err := s.repo.Grant(ctx, userID, teamID, grantedBy); err != nil {
		return err
	}

	// Drop any cached "not permitted" answer so the grant applies at once.
	s.authz.InvalidateUser(userID)
	return nil
}

func (s *teamEditorService) Revoke(ctx context.Context, userID, teamID string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	if err := s.repo.Revoke(ctx, userID, teamID); err != nil {
		return err
	}

	// Revocation must be immediate, so the cached grant goes now rather than
	// at the end of its TTL.
	s.authz.InvalidateUser(userID)
	return nil
}
