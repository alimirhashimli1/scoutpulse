package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/scoutpulse/football-svc/internal/domain"
)

// PlayerFilter narrows a player listing. A nil field means "no filter on this
// dimension", which is distinct from filtering on the zero value.
type PlayerFilter struct {
	FreeAgent *bool
	// IDs restricts the listing to specific players.
	//
	// This exists for one caller: the transfer feed, where every row carries a
	// player_id and no name. Resolving those one request at a time is an N+1 --
	// twenty-five rows, twenty-five round trips, and all of them during server
	// rendering of the landing page. One ANY() lookup replaces the lot.
	IDs         []string
	Position    *string
	TeamID      *string
	Nationality *string
	// MinValue and MaxValue bound the current market value, in minor units.
	MinValue *domain.Minor
	MaxValue *domain.Minor
}

type PlayerRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Player, error)
	List(ctx context.Context, filter PlayerFilter, page domain.Page) ([]domain.Player, error)
	Create(ctx context.Context, player *domain.Player) error
	Update(ctx context.Context, player *domain.Player) error
	Delete(ctx context.Context, id string) error
}

type postgresPlayerRepository struct {
	db *sqlx.DB
}

func NewPostgresPlayerRepository(db *sqlx.DB) PlayerRepository {
	return &postgresPlayerRepository{db: db}
}

const playerColumns = `id, team_id, name, first_name, last_name, date_of_birth,
	nationalities, height_cm, preferred_foot, agent, squad_number,
	position, secondary_positions, contract_start, contract_until,
	market_value_minor, currency,
	duels_won_pct, pass_completion_pct, shots_on_target_pct, aerial_duels_won_pct,
	strengths, weaknesses, created_at`

func (r *postgresPlayerRepository) GetByID(ctx context.Context, id string) (*domain.Player, error) {
	var player domain.Player
	query := `SELECT ` + playerColumns + ` FROM players WHERE id = $1`
	if err := r.db.GetContext(ctx, &player, query, id); err != nil {
		return nil, translate("player", err)
	}
	return &player, nil
}

func (r *postgresPlayerRepository) List(ctx context.Context, filter PlayerFilter, page domain.Page) ([]domain.Player, error) {
	var (
		conditions []string
		args       []any
	)

	// Placeholders are numbered as they are appended, so the WHERE clause and
	// the argument slice cannot drift apart.
	placeholder := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if len(filter.IDs) > 0 {
		conditions = append(conditions, "id = ANY("+placeholder(pq.Array(filter.IDs))+")")
	}
	if filter.FreeAgent != nil {
		if *filter.FreeAgent {
			conditions = append(conditions, "team_id IS NULL")
		} else {
			conditions = append(conditions, "team_id IS NOT NULL")
		}
	}
	if filter.Position != nil && *filter.Position != "" {
		conditions = append(conditions, "position = "+placeholder(*filter.Position))
	}
	if filter.TeamID != nil && *filter.TeamID != "" {
		conditions = append(conditions, "team_id = "+placeholder(*filter.TeamID))
	}
	if filter.Nationality != nil && *filter.Nationality != "" {
		// Containment, not equality: the column is a list now, and someone with
		// dual nationality must appear under either of them.
		conditions = append(conditions, "nationalities @> ARRAY["+placeholder(*filter.Nationality)+"]")
	}
	if filter.MinValue != nil {
		conditions = append(conditions, "market_value_minor >= "+placeholder(int64(*filter.MinValue)))
	}
	if filter.MaxValue != nil {
		conditions = append(conditions, "market_value_minor <= "+placeholder(int64(*filter.MaxValue)))
	}

	query := `SELECT ` + playerColumns + ` FROM players`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY name, id"
	query += " LIMIT " + placeholder(page.FetchLimit())
	query += " OFFSET " + placeholder(page.Offset)

	var players []domain.Player
	if err := r.db.SelectContext(ctx, &players, query, args...); err != nil {
		return nil, translate("player", err)
	}
	return players, nil
}

// Create inserts a player together with the history behind its derived state.
//
// The schema's rule is that transfers and player_market_values are the sources
// of truth and the columns on players are derived from them. Migration 000003
// backfilled an opening transfer and valuation for every pre-existing row for
// exactly that reason; inserting a player without them would reintroduce the
// gap the backfill closed -- a squad membership with an empty career history,
// and a current valuation missing from the valuation chart.
//
// All three rows go in one transaction: a player whose history half-exists is
// worse than one that failed to be created.
func (r *postgresPlayerRepository) Create(ctx context.Context, p *domain.Player) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translate("player", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := `INSERT INTO players
		(team_id, name, first_name, last_name, date_of_birth, nationalities,
		 height_cm, preferred_foot, agent, squad_number, position, secondary_positions,
		 contract_start, contract_until, market_value_minor, currency,
		 duels_won_pct, pass_completion_pct, shots_on_target_pct, aerial_duels_won_pct,
		 strengths, weaknesses)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
		        $17, $18, $19, $20, $21, $22)
		RETURNING id, created_at`
	if err := tx.QueryRowContext(ctx, query,
		p.TeamID, p.Name, p.FirstName, p.LastName, p.DateOfBirth, p.Nationalities,
		p.HeightCM, p.PreferredFoot, p.Agent, p.SquadNumber, p.Position, p.SecondaryPositions,
		p.ContractStart, p.ContractUntil, int64(p.MarketValue), p.Currency,
		p.DuelsWonPct, p.PassCompletionPct, p.ShotsOnTargetPct, p.AerialDuelsWonPct,
		p.Strengths, p.Weaknesses,
	).Scan(&p.ID, &p.CreatedAt); err != nil {
		return translate("player", err)
	}

	// The opening transfer: an arrival from nowhere at the club they joined
	// with. Free agents get none, because there is no move to record.
	if p.TeamID != nil {
		openingDate := p.ContractStart
		if openingDate == nil {
			openingDate = &p.CreatedAt
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO transfers (player_id, from_team_id, to_team_id, transfer_date, transfer_type)
			 VALUES ($1, NULL, $2, $3, $4)`,
			p.ID, *p.TeamID, *openingDate, string(domain.TransferPermanent),
		); err != nil {
			return translate("transfer", err)
		}
	}

	// The opening valuation, so the chart starts where the player does.
	if p.MarketValue > 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO player_market_values (player_id, valued_on, value_minor, currency)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (player_id, valued_on) DO NOTHING`,
			p.ID, p.CreatedAt, int64(p.MarketValue), p.Currency,
		); err != nil {
			return translate("market value", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return translate("player", err)
	}
	return nil
}

// Update writes the player's descriptive fields.
//
// team_id and market_value_minor are deliberately absent: they are derived
// state owned by the transfer and valuation flows. Letting a plain update
// change them would let the current club drift away from the transfer history.
func (r *postgresPlayerRepository) Update(ctx context.Context, p *domain.Player) error {
	query := `UPDATE players SET
		name = $1, first_name = $2, last_name = $3, date_of_birth = $4, nationalities = $5,
		height_cm = $6, preferred_foot = $7, agent = $8, squad_number = $9,
		position = $10, secondary_positions = $11, contract_start = $12, contract_until = $13,
		duels_won_pct = $14, pass_completion_pct = $15, shots_on_target_pct = $16,
		aerial_duels_won_pct = $17, strengths = $18, weaknesses = $19
		WHERE id = $20`
	res, err := r.db.ExecContext(ctx, query,
		p.Name, p.FirstName, p.LastName, p.DateOfBirth, p.Nationalities,
		p.HeightCM, p.PreferredFoot, p.Agent, p.SquadNumber,
		p.Position, p.SecondaryPositions, p.ContractStart, p.ContractUntil,
		p.DuelsWonPct, p.PassCompletionPct, p.ShotsOnTargetPct,
		p.AerialDuelsWonPct, p.Strengths, p.Weaknesses,
		p.ID,
	)
	return affected("player", res, err)
}

func (r *postgresPlayerRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM players WHERE id = $1`, id)
	return affected("player", res, err)
}
