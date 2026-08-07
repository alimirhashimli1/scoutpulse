package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

// PlayerFilter narrows a player listing. A nil field means "no filter on this
// dimension", which is distinct from filtering on the zero value.
type PlayerFilter struct {
	FreeAgent *bool
	Position  *string
	TeamID    *string
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

const playerColumns = `id, team_id, name, position, market_value, contract_until, created_at`

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

func (r *postgresPlayerRepository) Create(ctx context.Context, player *domain.Player) error {
	query := `INSERT INTO players (team_id, name, position, market_value, contract_until)
	          VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query,
		player.TeamID, player.Name, player.Position, player.MarketValue, player.ContractUntil).
		Scan(&player.ID, &player.CreatedAt)
	return translate("player", err)
}

func (r *postgresPlayerRepository) Update(ctx context.Context, player *domain.Player) error {
	query := `UPDATE players SET team_id = $1, name = $2, position = $3, market_value = $4, contract_until = $5
	          WHERE id = $6`
	res, err := r.db.ExecContext(ctx, query,
		player.TeamID, player.Name, player.Position, player.MarketValue, player.ContractUntil, player.ID)
	return affected("player", res, err)
}

func (r *postgresPlayerRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM players WHERE id = $1`, id)
	return affected("player", res, err)
}
