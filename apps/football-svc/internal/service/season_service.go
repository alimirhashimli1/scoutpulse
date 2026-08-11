package service

import (
	"context"
	"time"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
)

type SeasonService interface {
	GetSeason(ctx context.Context, id string) (*domain.Season, error)
	CurrentSeason(ctx context.Context) (*domain.Season, error)
	ListSeasons(ctx context.Context, page domain.Page) ([]domain.Season, error)
	CreateSeason(ctx context.Context, season *domain.Season) error
	UpdateSeason(ctx context.Context, season *domain.Season) error
	DeleteSeason(ctx context.Context, id string) error
}

type seasonService struct {
	repo  repository.SeasonRepository
	authz *Authorizer
}

func NewSeasonService(repo repository.SeasonRepository, authz *Authorizer) SeasonService {
	return &seasonService{repo: repo, authz: authz}
}

func (s *seasonService) GetSeason(ctx context.Context, id string) (*domain.Season, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *seasonService) CurrentSeason(ctx context.Context) (*domain.Season, error) {
	return s.repo.Current(ctx, time.Now().UTC())
}

func (s *seasonService) ListSeasons(ctx context.Context, page domain.Page) ([]domain.Season, error) {
	return s.repo.List(ctx, page)
}

func validateSeason(s *domain.Season) error {
	if s.Label == "" {
		return apperr.Invalid("label is required")
	}
	if s.StartDate.IsZero() || s.EndDate.IsZero() {
		return apperr.Invalid("start_date and end_date are required")
	}
	if !s.EndDate.After(s.StartDate) {
		return apperr.Invalid("end_date must be after start_date")
	}
	return nil
}

// checkNoOverlap rejects a season whose dates run into another's.
//
// Overlapping seasons make Current ambiguous: it resolves the tie by sort
// order, so a transfer would be filed against whichever season happened to
// sort first. The database enforces this too; checking here turns a constraint
// violation into a message that says which season is in the way.
func (s *seasonService) checkNoOverlap(ctx context.Context, season *domain.Season) error {
	existing, err := s.repo.Overlapping(ctx, season.StartDate, season.EndDate, season.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		return apperr.Conflict("these dates overlap season " + existing.Label)
	}
	return nil
}

func (s *seasonService) CreateSeason(ctx context.Context, season *domain.Season) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	if err := validateSeason(season); err != nil {
		return err
	}
	if err := s.checkNoOverlap(ctx, season); err != nil {
		return err
	}
	return s.repo.Create(ctx, season)
}

func (s *seasonService) UpdateSeason(ctx context.Context, season *domain.Season) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	if err := validateSeason(season); err != nil {
		return err
	}
	if err := s.checkNoOverlap(ctx, season); err != nil {
		return err
	}
	return s.repo.Update(ctx, season)
}

func (s *seasonService) DeleteSeason(ctx context.Context, id string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
