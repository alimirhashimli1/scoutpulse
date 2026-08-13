package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/platform/apperr"
)

// TransferFilter narrows a transfer listing. A nil field means no filter on
// that dimension.
type TransferFilter struct {
	PlayerID *string
	TeamID   *string // matches either side of the move
	SeasonID *string
	Type     *domain.TransferType
	// MinFee filters on the recorded fee. Undisclosed fees (NULL) are
	// excluded when this is set, since an unknown amount cannot be compared.
	MinFee *domain.Minor
}

type TransferRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Transfer, error)
	List(ctx context.Context, filter TransferFilter, page domain.Page) ([]domain.Transfer, error)
	// Record inserts the transfer and moves the player to the destination
	// club atomically. Both must happen or neither: a transfer row without
	// the matching squad change would leave the history disagreeing with the
	// current state.
	Record(ctx context.Context, transfer *domain.Transfer) error
	// Update corrects the descriptive fields of a recorded move: type, fee,
	// currency and season.
	//
	// The player, both clubs and the date are deliberately not writable here.
	// They are what players.team_id is derived from, so changing them would
	// have to re-run the origin check, the FOR UPDATE lock and the
	// latest-move guard that Record performs -- at which point it is the same
	// operation. Correcting those means deleting the row and filing it again.
	Update(ctx context.Context, transfer *domain.Transfer) error
	Delete(ctx context.Context, id string) error
}

type postgresTransferRepository struct {
	db *sqlx.DB
}

func NewPostgresTransferRepository(db *sqlx.DB) TransferRepository {
	return &postgresTransferRepository{db: db}
}

const transferColumns = `id, player_id, from_team_id, to_team_id, season_id,
	transfer_date, transfer_type, fee_minor, currency, created_at`

func (r *postgresTransferRepository) GetByID(ctx context.Context, id string) (*domain.Transfer, error) {
	var t domain.Transfer
	query := `SELECT ` + transferColumns + ` FROM transfers WHERE id = $1`
	if err := r.db.GetContext(ctx, &t, query, id); err != nil {
		return nil, translate("transfer", err)
	}
	return &t, nil
}

func (r *postgresTransferRepository) List(ctx context.Context, filter TransferFilter, page domain.Page) ([]domain.Transfer, error) {
	var (
		conditions []string
		args       []any
	)

	placeholder := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if filter.PlayerID != nil && *filter.PlayerID != "" {
		conditions = append(conditions, "player_id = "+placeholder(*filter.PlayerID))
	}
	if filter.TeamID != nil && *filter.TeamID != "" {
		// A club's transfer activity is both its arrivals and its departures.
		p := placeholder(*filter.TeamID)
		conditions = append(conditions, "(from_team_id = "+p+" OR to_team_id = "+p+")")
	}
	if filter.SeasonID != nil && *filter.SeasonID != "" {
		conditions = append(conditions, "season_id = "+placeholder(*filter.SeasonID))
	}
	if filter.Type != nil && *filter.Type != "" {
		conditions = append(conditions, "transfer_type = "+placeholder(string(*filter.Type)))
	}
	if filter.MinFee != nil {
		conditions = append(conditions, "fee_minor >= "+placeholder(int64(*filter.MinFee)))
	}

	query := `SELECT ` + transferColumns + ` FROM transfers`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	// Newest first: a transfer feed is read most-recent-first.
	query += " ORDER BY transfer_date DESC, id"
	query += " LIMIT " + placeholder(page.FetchLimit())
	query += " OFFSET " + placeholder(page.Offset)

	var transfers []domain.Transfer
	if err := r.db.SelectContext(ctx, &transfers, query, args...); err != nil {
		return nil, translate("transfer", err)
	}
	return transfers, nil
}

func (r *postgresTransferRepository) Record(ctx context.Context, t *domain.Transfer) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translate("transfer", err)
	}
	// Rolled back unless the commit below succeeds; a no-op after commit.
	defer func() { _ = tx.Rollback() }()

	// Lock the player and re-read their club inside the transaction.
	//
	// The service checked the origin against a read taken outside this
	// transaction. Without the lock, two concurrent transfers for the same
	// player both pass that check and both commit, leaving two moves recorded
	// from the same origin and whichever squad update landed last as the
	// current state.
	var currentTeam *string
	if err := tx.QueryRowContext(ctx,
		`SELECT team_id FROM players WHERE id = $1 FOR UPDATE`, t.PlayerID).Scan(&currentTeam); err != nil {
		return translate("player", err)
	}

	if !sameTeam(currentTeam, t.FromTeam) {
		return apperr.Invalid("the player's club changed while this transfer was being recorded; retry with the current from_team_id")
	}

	insert := `INSERT INTO transfers
		(player_id, from_team_id, to_team_id, season_id, transfer_date, transfer_type, fee_minor, currency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`
	err = tx.QueryRowContext(ctx, insert,
		t.PlayerID, t.FromTeam, t.ToTeam, t.SeasonID,
		t.TransferDate, string(t.Type), t.Fee, t.Currency,
	).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return translate("transfer", err)
	}

	// Move the player, but only when this is their latest move. A loan return
	// and a release both resolve to a plain destination, which may be NULL for
	// a player leaving the dataset.
	//
	// The NOT EXISTS guard is what makes backfilling safe: recording a move
	// from 2015 must not make a 2015 club the player's current one. The same
	// rule the valuation flow applies to an older figure.
	update := `UPDATE players p SET team_id = $1
	            WHERE p.id = $2
	              AND NOT EXISTS (
	                  SELECT 1 FROM transfers older
	                   WHERE older.player_id = p.id
	                     AND older.id <> $3
	                     AND older.transfer_date > $4
	              )`
	if _, err := tx.ExecContext(ctx, update, t.ToTeam, t.PlayerID, t.ID, t.TransferDate); err != nil {
		return translate("player", err)
	}

	if err := tx.Commit(); err != nil {
		return translate("transfer", err)
	}
	return nil
}

// sameTeam compares two optional club ids, treating nil as "no club".
func sameTeam(a, b *string) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func (r *postgresTransferRepository) Update(ctx context.Context, t *domain.Transfer) error {
	query := `UPDATE transfers
	             SET transfer_type = $1, fee_minor = $2, currency = $3, season_id = $4
	           WHERE id = $5`
	res, err := r.db.ExecContext(ctx, query,
		string(t.Type), t.Fee, t.Currency, t.SeasonID, t.ID)
	return affected("transfer", res, err)
}

func (r *postgresTransferRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM transfers WHERE id = $1`, id)
	return affected("transfer", res, err)
}
