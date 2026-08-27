package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/events"
)

type PlayerService interface {
	GetPlayer(ctx context.Context, id string) (*domain.Player, error)
	ListPlayersByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.Player, error)
	ListPlayers(ctx context.Context, filter repository.PlayerFilter, page domain.Page) ([]domain.Player, error)
	CreatePlayer(ctx context.Context, player *domain.Player) error
	UpdatePlayer(ctx context.Context, player *domain.Player) error
	DeletePlayer(ctx context.Context, id string) error
}

type playerService struct {
	repo      repository.PlayerRepository
	authz     *Authorizer
	publisher events.Publisher
}

func NewPlayerService(repo repository.PlayerRepository, authz *Authorizer, publisher events.Publisher) PlayerService {
	return &playerService{repo: repo, authz: authz, publisher: publisher}
}

func (s *playerService) GetPlayer(ctx context.Context, id string) (*domain.Player, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *playerService) ListPlayersByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.Player, error) {
	return s.repo.List(ctx, repository.PlayerFilter{TeamID: &teamID}, page)
}

// ListPlayers is a public read; no authorization check applies.
func (s *playerService) ListPlayers(ctx context.Context, filter repository.PlayerFilter, page domain.Page) ([]domain.Player, error) {
	return s.repo.List(ctx, filter, page)
}

func validatePlayer(p *domain.Player) error {
	if p.Name == "" {
		return apperr.Invalid("name is required")
	}
	if p.Position == "" {
		return apperr.Invalid("position is required")
	}
	if p.PreferredFoot != nil && !domain.ValidFoot(*p.PreferredFoot) {
		return apperr.Invalid("preferred_foot must be one of: left, right, both")
	}
	if p.HeightCM != nil && (*p.HeightCM < 100 || *p.HeightCM > 250) {
		return apperr.Invalid("height_cm must be between 100 and 250")
	}
	if p.SquadNumber != nil && (*p.SquadNumber < 1 || *p.SquadNumber > 99) {
		return apperr.Invalid("squad_number must be between 1 and 99")
	}
	if p.DateOfBirth != nil && p.DateOfBirth.After(time.Now()) {
		return apperr.Invalid("date_of_birth cannot be in the future")
	}
	if p.ContractStart != nil && p.ContractUntil != nil && p.ContractUntil.Before(*p.ContractStart) {
		return apperr.Invalid("contract_until must be on or after contract_start")
	}
	if p.MarketValue < 0 {
		return apperr.Invalid("market_value_minor cannot be negative")
	}
	if p.Currency == "" {
		p.Currency = domain.DefaultCurrency
	}

	// --- lists ---
	// Cleaned rather than rejected: a trailing blank entry from a form with an
	// empty last row is a UI artefact, not something to make the user fix.
	p.Nationalities = tidyList(p.Nationalities)
	p.SecondaryPositions = tidyList(p.SecondaryPositions)
	p.Strengths = tidyList(p.Strengths)
	p.Weaknesses = tidyList(p.Weaknesses)

	if len(p.Nationalities) > maxNationalities {
		return apperr.Invalid("at most 3 nationalities")
	}
	if len(p.SecondaryPositions) > maxSecondaryPositions {
		return apperr.Invalid("at most 6 secondary positions")
	}
	// The main position appearing again in the secondary list says nothing and
	// renders as a duplicate on the profile.
	for _, secondary := range p.SecondaryPositions {
		if strings.EqualFold(secondary, p.Position) {
			return apperr.Invalid("a secondary position cannot repeat the main position")
		}
	}
	if err := checkAssessment("strengths", p.Strengths); err != nil {
		return err
	}
	if err := checkAssessment("weaknesses", p.Weaknesses); err != nil {
		return err
	}

	// --- percentages ---
	for _, pct := range []struct {
		name  string
		value *float64
	}{
		{"duels_won_pct", p.DuelsWonPct},
		{"pass_completion_pct", p.PassCompletionPct},
		{"shots_on_target_pct", p.ShotsOnTargetPct},
		{"aerial_duels_won_pct", p.AerialDuelsWonPct},
	} {
		if pct.value == nil {
			continue
		}
		if *pct.value < 0 || *pct.value > 100 {
			return apperr.Invalid(pct.name + " must be between 0 and 100")
		}
	}

	return nil
}

// Bounds on the scouting lists. Generous, because the point of a strengths
// list is that a scout writes what they saw -- but bounded, because a text
// array with no ceiling is an unbounded write.
const (
	maxNationalities      = 3
	maxSecondaryPositions = 6
	maxAssessmentItems    = 12
	// Long enough for "Reads the game exceptionally well under pressure", short
	// enough that the list stays a list rather than becoming prose.
	maxAssessmentItemLength = 200
)

// checkAssessment bounds one of the strengths/weaknesses lists.
func checkAssessment(field string, items domain.StringList) error {
	if len(items) > maxAssessmentItems {
		return apperr.Invalid(fmt.Sprintf("at most %d %s", maxAssessmentItems, field))
	}
	for _, item := range items {
		// Runes, not bytes: a point written in Turkish is measured the way its
		// author counts it.
		if utf8.RuneCountInString(item) > maxAssessmentItemLength {
			return apperr.Invalid(fmt.Sprintf(
				"each entry in %s can be at most %d characters", field, maxAssessmentItemLength))
		}
	}
	return nil
}

// tidyList trims each entry and drops the empty ones, preserving order.
func tidyList(items domain.StringList) domain.StringList {
	cleaned := make(domain.StringList, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func (s *playerService) CreatePlayer(ctx context.Context, player *domain.Player) error {
	if err := s.authz.RequireTargetTeam(ctx, player.TeamID); err != nil {
		return err
	}
	if err := validatePlayer(player); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, player); err != nil {
		return err
	}

	_ = s.publisher.Publish(ctx, events.SubjectPlayerCreated, events.PlayerCreated{
		PlayerID: player.ID,
		Name:     player.Name,
		TeamID:   player.TeamID,
		Position: player.Position,
	})
	return nil
}

// UpdatePlayer edits a player's descriptive fields.
//
// It cannot move a player between clubs: that is a transfer, and it goes
// through TransferService so the move is recorded in the history rather than
// silently overwriting the current club. The authorization check therefore
// only needs the player's existing club.
func (s *playerService) UpdatePlayer(ctx context.Context, player *domain.Player) error {
	existing, err := s.repo.GetByID(ctx, player.ID)
	if err != nil {
		return err
	}

	if err := s.authz.RequireTargetTeam(ctx, existing.TeamID); err != nil {
		return err
	}
	if err := validatePlayer(player); err != nil {
		return err
	}

	// Preserve derived state the caller has no authority to set here.
	player.TeamID = existing.TeamID
	player.MarketValue = existing.MarketValue
	player.Currency = existing.Currency

	return s.repo.Update(ctx, player)
}

func (s *playerService) DeletePlayer(ctx context.Context, id string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
