package service

import (
	"testing"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// storedTransfer is the record every test here corrects.
func storedTransfer() *domain.Transfer {
	fee := domain.Minor(1_000_000)
	season := "season-1"
	return &domain.Transfer{
		ID:           "transfer-1",
		PlayerID:     "player-1",
		FromTeam:     ptr("team-selling"),
		ToTeam:       ptr("team-buying"),
		SeasonID:     &season,
		TransferDate: today(),
		Type:         domain.TransferPermanent,
		Fee:          &fee,
		Currency:     "EUR",
	}
}

// TestUpdateTransfer_CannotMoveThePlayer is the property that matters.
//
// players.team_id is derived from the transfer history, and Record maintains it
// behind a FOR UPDATE lock and a latest-move guard. An edit path that could
// change the clubs or the date would have to repeat all of that; instead those
// fields are taken from the stored row and the request's values are ignored.
func TestUpdateTransfer_CannotMoveThePlayer(t *testing.T) {
	svc, transfers, _, _ := newTransferFixture(t, newStubEditors(), nil)
	stored := storedTransfer()
	transfers.On("GetByID", mock.Anything, "transfer-1").Return(stored, nil)

	var written *domain.Transfer
	transfers.On("Update", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { written = args.Get(1).(*domain.Transfer) }).
		Return(nil).Once()

	// Every immutable field is given a different value on the way in.
	err := svc.UpdateTransfer(ctxAs("user-admin", RoleAdmin), &domain.Transfer{
		ID:           "transfer-1",
		PlayerID:     "someone-else",
		FromTeam:     ptr("team-elsewhere"),
		ToTeam:       ptr("team-hijacked"),
		TransferDate: today().AddDate(-3, 0, 0),
		Type:         domain.TransferLoan,
	})

	require.NoError(t, err)
	require.NotNil(t, written)

	assert.Equal(t, "player-1", written.PlayerID, "the player must come from the stored row")
	assert.Equal(t, "team-selling", *written.FromTeam, "the origin must come from the stored row")
	assert.Equal(t, "team-buying", *written.ToTeam, "the destination must come from the stored row")
	assert.Equal(t, stored.TransferDate, written.TransferDate, "the date must come from the stored row")

	// The editable field did change.
	assert.Equal(t, domain.TransferLoan, written.Type)
}

func TestUpdateTransfer_CorrectsTheFee(t *testing.T) {
	svc, transfers, _, _ := newTransferFixture(t, newStubEditors(), nil)
	transfers.On("GetByID", mock.Anything, "transfer-1").Return(storedTransfer(), nil)

	var written *domain.Transfer
	transfers.On("Update", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { written = args.Get(1).(*domain.Transfer) }).
		Return(nil).Once()

	corrected := domain.Minor(25_000_000_00)
	err := svc.UpdateTransfer(ctxAs("user-admin", RoleAdmin), &domain.Transfer{
		ID:   "transfer-1",
		Type: domain.TransferPermanent,
		Fee:  &corrected,
	})

	require.NoError(t, err)
	require.NotNil(t, written.Fee)
	assert.Equal(t, corrected, *written.Fee)
}

// TestUpdateTransfer_KeepsTheSeasonWhenOmitted: a caller correcting only the
// fee should not silently detach the record from its season.
func TestUpdateTransfer_KeepsTheSeasonWhenOmitted(t *testing.T) {
	svc, transfers, _, _ := newTransferFixture(t, newStubEditors(), nil)
	transfers.On("GetByID", mock.Anything, "transfer-1").Return(storedTransfer(), nil)

	var written *domain.Transfer
	transfers.On("Update", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { written = args.Get(1).(*domain.Transfer) }).
		Return(nil).Once()

	err := svc.UpdateTransfer(ctxAs("user-admin", RoleAdmin), &domain.Transfer{
		ID:   "transfer-1",
		Type: domain.TransferPermanent,
	})

	require.NoError(t, err)
	require.NotNil(t, written.SeasonID)
	assert.Equal(t, "season-1", *written.SeasonID)
}

// TestUpdateTransfer_EitherClubMayCorrect mirrors recording: the selling and
// buying club are both party to the move, so either editor may fix it.
func TestUpdateTransfer_EitherClubMayCorrect(t *testing.T) {
	for _, club := range []string{"team-selling", "team-buying"} {
		t.Run(club, func(t *testing.T) {
			editors := newStubEditors().grant("user-editor", club)
			svc, transfers, _, _ := newTransferFixture(t, editors, nil)
			transfers.On("GetByID", mock.Anything, "transfer-1").Return(storedTransfer(), nil)
			transfers.On("Update", mock.Anything, mock.Anything).Return(nil).Once()

			err := svc.UpdateTransfer(ctxAs("user-editor", RoleEditor), &domain.Transfer{
				ID: "transfer-1", Type: domain.TransferPermanent,
			})

			assert.NoError(t, err)
		})
	}
}

func TestUpdateTransfer_UnrelatedEditorIsRefused(t *testing.T) {
	editors := newStubEditors().grant("user-editor", "team-unrelated")
	svc, transfers, _, _ := newTransferFixture(t, editors, nil)
	transfers.On("GetByID", mock.Anything, "transfer-1").Return(storedTransfer(), nil)

	err := svc.UpdateTransfer(ctxAs("user-editor", RoleEditor), &domain.Transfer{
		ID: "transfer-1", Type: domain.TransferPermanent,
	})

	assert.ErrorIs(t, err, ErrForbidden)
	transfers.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

// TestUpdateTransfer_StillValidates: the input rules apply to a correction
// exactly as they do to the original.
func TestUpdateTransfer_StillValidates(t *testing.T) {
	tests := []struct {
		name  string
		patch domain.Transfer
	}{
		{"unknown type", domain.Transfer{ID: "transfer-1", Type: "teleported"}},
		{"negative fee", domain.Transfer{ID: "transfer-1", Type: domain.TransferPermanent, Fee: ptr(domain.Minor(-1))}},
		{"free transfer with a fee", domain.Transfer{ID: "transfer-1", Type: domain.TransferFree, Fee: ptr(domain.Minor(500))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, transfers, _, _ := newTransferFixture(t, newStubEditors(), nil)
			transfers.On("GetByID", mock.Anything, "transfer-1").Return(storedTransfer(), nil)

			err := svc.UpdateTransfer(ctxAs("user-admin", RoleAdmin), &tt.patch)

			require.Error(t, err)
			assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
			transfers.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		})
	}
}
