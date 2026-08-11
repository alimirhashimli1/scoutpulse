package service

import (
	"context"
	"testing"
	"time"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockTransferRepository struct{ mock.Mock }

func (m *MockTransferRepository) GetByID(ctx context.Context, id string) (*domain.Transfer, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Transfer), args.Error(1)
}

func (m *MockTransferRepository) List(ctx context.Context, f repository.TransferFilter, p domain.Page) ([]domain.Transfer, error) {
	args := m.Called(ctx, f, p)
	return args.Get(0).([]domain.Transfer), args.Error(1)
}

func (m *MockTransferRepository) Record(ctx context.Context, t *domain.Transfer) error {
	return m.Called(ctx, t).Error(0)
}

func (m *MockTransferRepository) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

// stubSeasons resolves the season a transfer date falls in. It returns a fixed
// season, or none at all when the fixture has no seasons defined.
type stubSeasons struct {
	season *domain.Season
}

func (s stubSeasons) GetByID(context.Context, string) (*domain.Season, error) {
	return s.season, nil
}

func (s stubSeasons) Current(context.Context, time.Time) (*domain.Season, error) {
	if s.season == nil {
		return nil, apperr.NotFound("season not found")
	}
	return s.season, nil
}

func (s stubSeasons) List(context.Context, domain.Page) ([]domain.Season, error) { return nil, nil }
func (s stubSeasons) Create(context.Context, *domain.Season) error               { return nil }
func (s stubSeasons) Update(context.Context, *domain.Season) error               { return nil }
func (s stubSeasons) Delete(context.Context, string) error                       { return nil }

// Overlapping reports no conflict: these fixtures define at most one season.
func (s stubSeasons) Overlapping(context.Context, time.Time, time.Time, string) (*domain.Season, error) {
	return nil, nil
}

func newTransferFixture(t *testing.T, editors *stubEditors, season *domain.Season) (
	TransferService, *MockTransferRepository, *MockPlayerRepository, *recordingPublisher,
) {
	t.Helper()
	transfers := new(MockTransferRepository)
	players := new(MockPlayerRepository)
	publisher := &recordingPublisher{}
	svc := NewTransferService(transfers, players, stubSeasons{season: season},
		newTestAuthorizer(t, editors), publisher)
	return svc, transfers, players, publisher
}

// TestRecordTransfer_EitherClubMayRecord is what makes transfers workable: the
// selling club and the buying club are describing the same event, so an editor
// on either side can enter it.
func TestRecordTransfer_EitherClubMayRecord(t *testing.T) {
	for _, granted := range []string{"team-from", "team-to"} {
		t.Run("editor of "+granted, func(t *testing.T) {
			editors := newStubEditors().grant("user-1", granted)
			svc, transfers, players, publisher := newTransferFixture(t, editors, nil)

			players.On("GetByID", mock.Anything, "player-1").
				Return(&domain.Player{ID: "player-1", TeamID: ptr("team-from"), Name: "Mover"}, nil).Once()
			transfers.On("Record", mock.Anything, mock.Anything).Return(nil).Once()

			err := svc.RecordTransfer(ctxAs("user-1", RoleEditor), &domain.Transfer{
				PlayerID:     "player-1",
				FromTeam:     ptr("team-from"),
				ToTeam:       ptr("team-to"),
				TransferDate: today(),
				Type:         domain.TransferPermanent,
				Fee:          ptr(domain.Minor(5_000_000)),
			})

			require.NoError(t, err)
			assert.True(t, publisher.published("football.player.transferred"))
		})
	}
}

func TestRecordTransfer_UnrelatedEditorIsRefused(t *testing.T) {
	editors := newStubEditors().grant("user-1", "team-elsewhere")
	svc, transfers, players, _ := newTransferFixture(t, editors, nil)

	players.On("GetByID", mock.Anything, "player-1").
		Return(&domain.Player{ID: "player-1", TeamID: ptr("team-from"), Name: "Mover"}, nil).Once()

	err := svc.RecordTransfer(ctxAs("user-1", RoleEditor), &domain.Transfer{
		PlayerID:     "player-1",
		FromTeam:     ptr("team-from"),
		ToTeam:       ptr("team-to"),
		TransferDate: today(),
		Type:         domain.TransferPermanent,
	})

	assert.ErrorIs(t, err, ErrForbidden)
	transfers.AssertNotCalled(t, "Record", mock.Anything, mock.Anything)
}

// TestRecordTransfer_OriginMustMatch stops the history contradicting the
// current squad: a move can only start where the player actually is.
func TestRecordTransfer_OriginMustMatch(t *testing.T) {
	tests := []struct {
		name        string
		currentTeam *string
		fromTeam    *string
	}{
		{"claims a club the player is not at", ptr("team-a"), ptr("team-b")},
		{"omits origin for a contracted player", ptr("team-a"), nil},
		{"states an origin for a free agent", nil, ptr("team-a")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, transfers, players, _ := newTransferFixture(t, newStubEditors(), nil)

			players.On("GetByID", mock.Anything, "player-1").
				Return(&domain.Player{ID: "player-1", TeamID: tt.currentTeam, Name: "P"}, nil).Once()

			err := svc.RecordTransfer(ctxAs("user-admin", RoleAdmin), &domain.Transfer{
				PlayerID:     "player-1",
				FromTeam:     tt.fromTeam,
				ToTeam:       ptr("team-new"),
				TransferDate: today(),
				Type:         domain.TransferPermanent,
			})

			require.Error(t, err)
			assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
			transfers.AssertNotCalled(t, "Record", mock.Anything, mock.Anything)
		})
	}
}

func TestRecordTransfer_Validation(t *testing.T) {
	tests := []struct {
		name     string
		transfer domain.Transfer
	}{
		{"no player", domain.Transfer{ToTeam: ptr("t"), TransferDate: today(), Type: domain.TransferPermanent}},
		{"no date", domain.Transfer{PlayerID: "p", ToTeam: ptr("t"), Type: domain.TransferPermanent}},
		{"unknown type", domain.Transfer{PlayerID: "p", ToTeam: ptr("t"), TransferDate: today(), Type: "teleported"}},
		{"no direction", domain.Transfer{PlayerID: "p", TransferDate: today(), Type: domain.TransferPermanent}},
		{"same club both sides", domain.Transfer{PlayerID: "p", FromTeam: ptr("t"), ToTeam: ptr("t"), TransferDate: today(), Type: domain.TransferPermanent}},
		{"negative fee", domain.Transfer{PlayerID: "p", ToTeam: ptr("t"), TransferDate: today(), Type: domain.TransferPermanent, Fee: ptr(domain.Minor(-1))}},
		{"free transfer with a fee", domain.Transfer{PlayerID: "p", ToTeam: ptr("t"), TransferDate: today(), Type: domain.TransferFree, Fee: ptr(domain.Minor(100))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, transfers, _, _ := newTransferFixture(t, newStubEditors(), nil)

			err := svc.RecordTransfer(ctxAs("user-admin", RoleAdmin), &tt.transfer)

			require.Error(t, err)
			assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
			transfers.AssertNotCalled(t, "Record", mock.Anything, mock.Anything)
		})
	}
}

// TestRecordTransfer_AttachesSeason checks a move is filed against the season
// containing its date, which is what makes the history queryable by season.
func TestRecordTransfer_AttachesSeason(t *testing.T) {
	season := &domain.Season{ID: "season-2526", Label: "2025/26"}
	svc, transfers, players, _ := newTransferFixture(t, newStubEditors(), season)

	players.On("GetByID", mock.Anything, "player-1").
		Return(&domain.Player{ID: "player-1", TeamID: nil, Name: "Free Agent"}, nil).Once()

	var recorded *domain.Transfer
	transfers.On("Record", mock.Anything, mock.Anything).Return(nil).Once().
		Run(func(args mock.Arguments) { recorded = args.Get(1).(*domain.Transfer) })

	transfer := &domain.Transfer{
		PlayerID:     "player-1",
		ToTeam:       ptr("team-new"),
		TransferDate: today(),
		Type:         domain.TransferFree,
	}
	require.NoError(t, svc.RecordTransfer(ctxAs("user-admin", RoleAdmin), transfer))

	require.NotNil(t, recorded)
	require.NotNil(t, recorded.SeasonID)
	assert.Equal(t, "season-2526", *recorded.SeasonID)
	assert.Equal(t, domain.DefaultCurrency, recorded.Currency, "currency should default")
}

// TestRecordTransfer_SucceedsWithoutSeason: historical data may predate any
// season that has been entered, and that must not block recording it.
func TestRecordTransfer_SucceedsWithoutSeason(t *testing.T) {
	svc, transfers, players, _ := newTransferFixture(t, newStubEditors(), nil)

	players.On("GetByID", mock.Anything, "player-1").
		Return(&domain.Player{ID: "player-1", TeamID: nil, Name: "P"}, nil).Once()
	transfers.On("Record", mock.Anything, mock.Anything).Return(nil).Once()

	err := svc.RecordTransfer(ctxAs("user-admin", RoleAdmin), &domain.Transfer{
		PlayerID:     "player-1",
		ToTeam:       ptr("team-new"),
		TransferDate: today(),
		Type:         domain.TransferFree,
	})

	assert.NoError(t, err)
}

func TestDeleteTransfer_IsAdminOnly(t *testing.T) {
	editors := newStubEditors().grant("user-1", "team-1")
	svc, transfers, _, _ := newTransferFixture(t, editors, nil)

	assert.ErrorIs(t, svc.DeleteTransfer(ctxAs("user-1", RoleEditor), "transfer-1"), ErrForbidden)
	transfers.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)

	transfers.On("Delete", mock.Anything, "transfer-1").Return(nil).Once()
	assert.NoError(t, svc.DeleteTransfer(ctxAs("user-admin", RoleAdmin), "transfer-1"))
}
