package service

import (
	"context"
	"time"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/events"
)

type TransferService interface {
	GetTransfer(ctx context.Context, id string) (*domain.Transfer, error)
	ListTransfers(ctx context.Context, filter repository.TransferFilter, page domain.Page) ([]domain.Transfer, error)
	// RecordTransfer registers a move and updates the player's current club.
	RecordTransfer(ctx context.Context, transfer *domain.Transfer) error
	// UpdateTransfer corrects a recorded move's type, fee, currency or season.
	UpdateTransfer(ctx context.Context, transfer *domain.Transfer) error
	DeleteTransfer(ctx context.Context, id string) error
}

type transferService struct {
	repo      repository.TransferRepository
	players   repository.PlayerRepository
	seasons   repository.SeasonRepository
	authz     *Authorizer
	publisher events.Publisher
}

func NewTransferService(
	repo repository.TransferRepository,
	players repository.PlayerRepository,
	seasons repository.SeasonRepository,
	authz *Authorizer,
	publisher events.Publisher,
) TransferService {
	return &transferService{
		repo:      repo,
		players:   players,
		seasons:   seasons,
		authz:     authz,
		publisher: publisher,
	}
}

func (s *transferService) GetTransfer(ctx context.Context, id string) (*domain.Transfer, error) {
	return s.repo.GetByID(ctx, id)
}

// ListTransfers is a public read: transfer history is the point of the product.
func (s *transferService) ListTransfers(ctx context.Context, filter repository.TransferFilter, page domain.Page) ([]domain.Transfer, error) {
	return s.repo.List(ctx, filter, page)
}

func validateTransfer(t *domain.Transfer) error {
	if t.PlayerID == "" {
		return apperr.Invalid("player_id is required")
	}
	if t.TransferDate.IsZero() {
		return apperr.Invalid("transfer_date is required")
	}
	if t.TransferDate.After(time.Now().AddDate(1, 0, 0)) {
		return apperr.Invalid("transfer_date cannot be more than a year in the future")
	}
	if !domain.ValidTransferType(t.Type) {
		return apperr.Invalid("transfer_type must be one of: permanent, loan, loan_return, free, youth_promotion, released, retired")
	}
	// A move with neither origin nor destination says nothing.
	if t.FromTeam == nil && t.ToTeam == nil {
		return apperr.Invalid("a transfer needs from_team_id, to_team_id, or both")
	}
	if t.FromTeam != nil && t.ToTeam != nil && *t.FromTeam == *t.ToTeam {
		return apperr.Invalid("from_team_id and to_team_id must differ")
	}
	if t.Fee != nil && *t.Fee < 0 {
		return apperr.Invalid("fee_minor cannot be negative")
	}
	// "Free" means the fee is known to be zero. Leaving it nil would mean
	// undisclosed, which contradicts the type.
	if t.Type == domain.TransferFree && t.Fee != nil && *t.Fee != 0 {
		return apperr.Invalid("a free transfer cannot carry a fee")
	}
	if t.Currency == "" {
		t.Currency = domain.DefaultCurrency
	}
	return nil
}

func (s *transferService) RecordTransfer(ctx context.Context, t *domain.Transfer) error {
	if err := validateTransfer(t); err != nil {
		return err
	}

	player, err := s.players.GetByID(ctx, t.PlayerID)
	if err != nil {
		return err
	}

	// The stated origin must match where the player actually is, or the
	// history would contradict the current squad.
	if err := checkOrigin(player, t); err != nil {
		return err
	}

	// Either club's editor may record the move: the selling club releasing a
	// player and the buying club signing one are the same event.
	if err := s.authz.RequireEitherTeam(ctx, player.TeamID, t.ToTeam); err != nil {
		return err
	}

	// Attach the season containing the transfer date, when one is defined.
	// A missing season is not an error: historical data may predate the
	// seasons that have been entered.
	if t.SeasonID == nil {
		if season, err := s.seasons.Current(ctx, t.TransferDate); err == nil {
			t.SeasonID = &season.ID
		} else if apperr.KindOf(err) != apperr.KindNotFound {
			return err
		}
	}

	if err := s.repo.Record(ctx, t); err != nil {
		return err
	}

	// Published after the commit: the move is a fact by this point, so a
	// delivery failure must not undo it.
	fee := (*int64)(nil)
	if t.Fee != nil {
		v := int64(*t.Fee)
		fee = &v
	}
	_ = s.publisher.Publish(ctx, events.SubjectPlayerTransferred, events.PlayerTransferred{
		TransferID:   t.ID,
		PlayerID:     t.PlayerID,
		PlayerName:   player.Name,
		FromTeamID:   t.FromTeam,
		ToTeamID:     t.ToTeam,
		TransferType: string(t.Type),
		FeeMinor:     fee,
		Currency:     t.Currency,
		TransferDate: t.TransferDate,
	})

	return nil
}

// checkOrigin verifies the transfer's stated origin against the player's
// current club.
func checkOrigin(player *domain.Player, t *domain.Transfer) error {
	switch {
	case t.FromTeam == nil && player.TeamID != nil:
		return apperr.Invalid("player is currently at a club; set from_team_id to that club")
	case t.FromTeam != nil && player.TeamID == nil:
		return apperr.Invalid("player is a free agent; omit from_team_id")
	case t.FromTeam != nil && player.TeamID != nil && *t.FromTeam != *player.TeamID:
		return apperr.Invalid("from_team_id does not match the player's current club")
	default:
		return nil
	}
}

// UpdateTransfer corrects a recorded move.
//
// Only the descriptive fields change: type, fee, currency and season. The
// player, both clubs and the date are preserved from the stored row, because
// they are what the player's current club is derived from — see the note on
// repository.TransferRepository.Update. A client that round-trips a GET into a
// PUT therefore cannot move anyone by accident.
//
// Authorization matches recording: either club's editor may correct a move
// they were entitled to file in the first place.
func (s *transferService) UpdateTransfer(ctx context.Context, t *domain.Transfer) error {
	if t.ID == "" {
		return apperr.Invalid("transfer id is required")
	}

	existing, err := s.repo.GetByID(ctx, t.ID)
	if err != nil {
		return err
	}

	if err := s.authz.RequireEitherTeam(ctx, existing.FromTeam, existing.ToTeam); err != nil {
		return err
	}

	// Immutable fields come from the stored row, never the request.
	t.PlayerID = existing.PlayerID
	t.FromTeam = existing.FromTeam
	t.ToTeam = existing.ToTeam
	t.TransferDate = existing.TransferDate
	t.CreatedAt = existing.CreatedAt

	if err := validateTransfer(t); err != nil {
		return err
	}

	// A season the caller did not supply keeps the one already on the record,
	// rather than being silently cleared.
	if t.SeasonID == nil {
		t.SeasonID = existing.SeasonID
	}

	return s.repo.Update(ctx, t)
}

// DeleteTransfer removes a mistaken record. It is admin-only and deliberately
// does not rewind the player's current club: correcting history and correcting
// the present are separate decisions, and guessing at the previous club from a
// deleted row would often be wrong.
func (s *transferService) DeleteTransfer(ctx context.Context, id string) error {
	if err := s.authz.RequireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
