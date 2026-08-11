package service

import (
	"context"
	"time"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/events"
)

type MarketValueService interface {
	ListValues(ctx context.Context, playerID string, page domain.Page) ([]domain.MarketValue, error)
	RecordValue(ctx context.Context, value *domain.MarketValue) error
	DeleteValue(ctx context.Context, playerID, id string) error
}

type marketValueService struct {
	repo      repository.MarketValueRepository
	players   repository.PlayerRepository
	authz     *Authorizer
	publisher events.Publisher
}

func NewMarketValueService(
	repo repository.MarketValueRepository,
	players repository.PlayerRepository,
	authz *Authorizer,
	publisher events.Publisher,
) MarketValueService {
	return &marketValueService{repo: repo, players: players, authz: authz, publisher: publisher}
}

// ListValues is a public read: valuation history is core product data.
func (s *marketValueService) ListValues(ctx context.Context, playerID string, page domain.Page) ([]domain.MarketValue, error) {
	return s.repo.ListByPlayer(ctx, playerID, page)
}

// RecordValue is admin-only.
//
// A market value is an editorial judgement about a player, not a fact about a
// club, so a club's editor has no standing to set it -- otherwise every editor
// could inflate their own squad's valuations.
func (s *marketValueService) RecordValue(ctx context.Context, v *domain.MarketValue) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}

	if v.PlayerID == "" {
		return apperr.Invalid("player_id is required")
	}
	if v.Value < 0 {
		return apperr.Invalid("value_minor cannot be negative")
	}
	if v.ValuedOn.IsZero() {
		v.ValuedOn = time.Now().UTC().Truncate(24 * time.Hour)
	}
	if v.ValuedOn.After(time.Now()) {
		return apperr.Invalid("valued_on cannot be in the future")
	}
	if v.Currency == "" {
		v.Currency = domain.DefaultCurrency
	}

	player, err := s.players.GetByID(ctx, v.PlayerID)
	if err != nil {
		return err
	}
	previous := player.MarketValue

	if err := s.repo.Record(ctx, v); err != nil {
		return err
	}

	_ = s.publisher.Publish(ctx, events.SubjectPlayerValueChanged, events.PlayerValueChanged{
		PlayerID:      v.PlayerID,
		PreviousMinor: int64(previous),
		NewMinor:      int64(v.Value),
		Currency:      v.Currency,
		ValuedOn:      v.ValuedOn,
	})

	return nil
}

func (s *marketValueService) DeleteValue(ctx context.Context, playerID, id string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	if playerID == "" || id == "" {
		return apperr.Invalid("player id and value id are required")
	}
	return s.repo.Delete(ctx, playerID, id)
}
