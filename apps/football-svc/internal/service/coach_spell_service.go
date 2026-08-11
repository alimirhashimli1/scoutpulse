package service

import (
	"context"
	"time"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/events"
)

type CoachSpellService interface {
	ListByCoach(ctx context.Context, coachID string, page domain.Page) ([]domain.CoachSpell, error)
	ListByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.CoachSpell, error)
	RecordSpell(ctx context.Context, spell *domain.CoachSpell) error
	DeleteSpell(ctx context.Context, id string) error
}

type coachSpellService struct {
	repo      repository.CoachSpellRepository
	coaches   repository.CoachRepository
	authz     *Authorizer
	publisher events.Publisher
}

func NewCoachSpellService(
	repo repository.CoachSpellRepository,
	coaches repository.CoachRepository,
	authz *Authorizer,
	publisher events.Publisher,
) CoachSpellService {
	return &coachSpellService{repo: repo, coaches: coaches, authz: authz, publisher: publisher}
}

func (s *coachSpellService) ListByCoach(ctx context.Context, coachID string, page domain.Page) ([]domain.CoachSpell, error) {
	return s.repo.ListByCoach(ctx, coachID, page)
}

func (s *coachSpellService) ListByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.CoachSpell, error) {
	return s.repo.ListByTeam(ctx, teamID, page)
}

func (s *coachSpellService) RecordSpell(ctx context.Context, spell *domain.CoachSpell) error {
	if spell.CoachID == "" {
		return apperr.Invalid("coach_id is required")
	}
	if spell.StartDate.IsZero() {
		return apperr.Invalid("start_date is required")
	}
	if spell.EndDate != nil && spell.EndDate.Before(spell.StartDate) {
		return apperr.Invalid("end_date must be on or after start_date")
	}
	// A start date years ahead is a typo, not a signing. The same bound the
	// transfer flow applies.
	if spell.StartDate.After(time.Now().AddDate(1, 0, 0)) {
		return apperr.Invalid("start_date cannot be more than a year in the future")
	}
	if spell.Role == "" {
		spell.Role = domain.SpellHeadCoach
	}
	// Validated against a known set, as transfer_type and preferred_foot are.
	// Without this any string was stored, so "asdf" became a coaching role.
	if !domain.ValidSpellRole(spell.Role) {
		return apperr.Invalid("role must be one of: head_coach, assistant_coach, interim_coach, caretaker, director_of_football, youth_coach")
	}

	coach, err := s.coaches.GetByID(ctx, spell.CoachID)
	if err != nil {
		return err
	}

	// As with a transfer, either the club losing the coach or the club
	// gaining them may record the appointment.
	if err := s.authz.RequireEitherTeam(ctx, coach.TeamID, spell.TeamID); err != nil {
		return err
	}

	if err := s.repo.Record(ctx, spell); err != nil {
		return err
	}

	_ = s.publisher.Publish(ctx, events.SubjectCoachAppointed, events.CoachAppointed{
		SpellID:   spell.ID,
		CoachID:   spell.CoachID,
		CoachName: coach.Name,
		TeamID:    spell.TeamID,
		Role:      string(spell.Role),
		StartDate: spell.StartDate,
	})

	return nil
}

func (s *coachSpellService) DeleteSpell(ctx context.Context, id string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
