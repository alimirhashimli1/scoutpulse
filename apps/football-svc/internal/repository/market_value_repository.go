package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

type MarketValueRepository interface {
	// ListByPlayer returns a player's valuation history, newest first.
	ListByPlayer(ctx context.Context, playerID string, page domain.Page) ([]domain.MarketValue, error)
	// Record stores a valuation and updates the player's current value
	// atomically. Re-recording the same day replaces that day's entry.
	Record(ctx context.Context, value *domain.MarketValue) error
	// Delete removes one valuation belonging to playerID and resets the
	// player's current value to whatever survives. Both arguments are required:
	// the id alone would let a request delete a valuation belonging to a
	// different player than the one its URL names.
	Delete(ctx context.Context, playerID, id string) error
}

type postgresMarketValueRepository struct {
	db *sqlx.DB
}

func NewPostgresMarketValueRepository(db *sqlx.DB) MarketValueRepository {
	return &postgresMarketValueRepository{db: db}
}

const marketValueColumns = `id, player_id, valued_on, value_minor, currency, created_at`

func (r *postgresMarketValueRepository) ListByPlayer(ctx context.Context, playerID string, page domain.Page) ([]domain.MarketValue, error) {
	var values []domain.MarketValue
	query := `SELECT ` + marketValueColumns + ` FROM player_market_values
	          WHERE player_id = $1 ORDER BY valued_on DESC, id LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &values, query, playerID, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("market value", err)
	}
	return values, nil
}

func (r *postgresMarketValueRepository) Record(ctx context.Context, v *domain.MarketValue) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translate("market value", err)
	}
	defer func() { _ = tx.Rollback() }()

	// One valuation per player per day. Re-recording corrects that day rather
	// than accumulating duplicates.
	insert := `INSERT INTO player_market_values (player_id, valued_on, value_minor, currency)
	           VALUES ($1, $2, $3, $4)
	           ON CONFLICT (player_id, valued_on)
	           DO UPDATE SET value_minor = EXCLUDED.value_minor, currency = EXCLUDED.currency
	           RETURNING id, created_at`
	if err := tx.QueryRowContext(ctx, insert, v.PlayerID, v.ValuedOn, int64(v.Value), v.Currency).
		Scan(&v.ID, &v.CreatedAt); err != nil {
		return translate("market value", err)
	}

	// Keep the denormalised current value in step, but only when this entry
	// is the most recent one -- backfilling an older valuation must not
	// overwrite a newer figure.
	update := `UPDATE players p
	              SET market_value_minor = $1, currency = $2
	            WHERE p.id = $3
	              AND NOT EXISTS (
	                  SELECT 1 FROM player_market_values mv
	                   WHERE mv.player_id = p.id AND mv.valued_on > $4
	              )`
	if _, err := tx.ExecContext(ctx, update, int64(v.Value), v.Currency, v.PlayerID, v.ValuedOn); err != nil {
		return translate("market value", err)
	}

	if err := tx.Commit(); err != nil {
		return translate("market value", err)
	}
	return nil
}

func (r *postgresMarketValueRepository) Delete(ctx context.Context, playerID, id string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translate("market value", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Scoped to the player named in the path, so a valuation cannot be deleted
	// through another player's URL.
	res, err := tx.ExecContext(ctx,
		`DELETE FROM player_market_values WHERE id = $1 AND player_id = $2`, id, playerID)
	if err := affected("market value", res, err); err != nil {
		return err
	}

	// Record maintains players.market_value_minor as the latest valuation, so
	// a delete has to as well -- otherwise removing the most recent entry
	// leaves the player advertising a figure with no history row behind it.
	// COALESCE handles deleting the only valuation there was.
	reset := `UPDATE players p SET
	              market_value_minor = COALESCE(latest.value_minor, 0),
	              currency           = COALESCE(latest.currency, p.currency)
	          FROM (SELECT 1) AS _
	          LEFT JOIN LATERAL (
	              SELECT mv.value_minor, mv.currency
	                FROM player_market_values mv
	               WHERE mv.player_id = $1
	               ORDER BY mv.valued_on DESC, mv.id DESC
	               LIMIT 1
	          ) AS latest ON TRUE
	          WHERE p.id = $1`
	if _, err := tx.ExecContext(ctx, reset, playerID); err != nil {
		return translate("market value", err)
	}

	if err := tx.Commit(); err != nil {
		return translate("market value", err)
	}
	return nil
}
