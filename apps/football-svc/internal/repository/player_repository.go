package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"football-database-app/apps/football-svc/internal/domain"
)

// This part of the file has a duplicate package repository declaration and imports.
// The following SEARCH/REPLACE block will fix this by removing the duplicate and updating the existing one.
// The existing `package repository` and its imports were already correct due to the previous test changes,
// but the duplicate section also needs updating.

// Original section to fix:
// package repository
//
// import (
// 	"context"
// 	"fmt"
// 	"strings"
//
// 	"github.com/jmoiron/sqlx"
// 	"github.com/scoutpulse/football-svc/internal/domain"
// )

// After fixing the duplicate, the correct import should be:
// package repository
//
// import (
// 	"context"
// 	"fmt"
// 	"strings"
//
// 	"github.com/jmoiron/sqlx"
// 	"football-database-app/apps/football-svc/internal/domain" // Corrected path
// )
)

type PlayerRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Player, error)
	ListByTeam(ctx context.Context, teamID string) ([]domain.Player, error)
	ListPlayers(ctx context.Context, freeAgent *bool, position *string) ([]domain.Player, error) // New method
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

func (r *postgresPlayerRepository) GetByID(ctx context.Context, id string) (*domain.Player, error) {
	var player domain.Player
	query := `SELECT id, team_id, name, position, market_value, contract_until, created_at FROM players WHERE id = $1`
	err := r.db.GetContext(ctx, &player, query, id)
	if err != nil {
		return nil, err
	}
	return &player, nil
}

func (r *postgresPlayerRepository) ListByTeam(ctx context.Context, teamID string) ([]domain.Player, error) {
	var players []domain.Player
	query := `SELECT id, team_id, name, position, market_value, contract_until, created_at FROM players WHERE team_id = $1`
	err := r.db.SelectContext(ctx, &players, query, teamID)
	if err != nil {
		return nil, err
	}
	return players, nil
}

// ListPlayers implements the new method for filtering players based on free agent status and position.
func (r *postgresPlayerRepository) ListPlayers(ctx context.Context, freeAgent *bool, position *string) ([]domain.Player, error) {
	var players []domain.Player
	query := `SELECT id, team_id, name, position, market_value, contract_until, created_at FROM players`

	conditions := []string{}
	args := []interface{}{}
	argCounter := 1

	if freeAgent != nil {
		if *freeAgent {
			conditions = append(conditions, fmt.Sprintf("team_id IS NULL"))
		} else {
			conditions = append(conditions, fmt.Sprintf("team_id IS NOT NULL"))
		}
	}

	if position != nil && *position != "" {
		conditions = append(conditions, fmt.Sprintf("position = $%d", argCounter))
		args = append(args, *position)
		argCounter++
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	err := r.db.SelectContext(ctx, &players, query, args...)
	if err != nil {
		return nil, err
	}
	return players, nil
}

func (r *postgresPlayerRepository) Create(ctx context.Context, player *domain.Player) error {
	query := `INSERT INTO players (team_id, name, position, market_value, contract_until) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, player.TeamID, player.Name, player.Position, player.MarketValue, player.ContractUntil).Scan(&player.ID, &player.CreatedAt)
	return err
}

func (r *postgresPlayerRepository) Update(ctx context.Context, player *domain.Player) error {
	query := `UPDATE players SET team_id = $1, name = $2, position = $3, market_value = $4, contract_until = $5 WHERE id = $6`
	_, err := r.db.ExecContext(ctx, query, player.TeamID, player.Name, player.Position, player.MarketValue, player.ContractUntil, player.ID)
	return err
}

func (r *postgresPlayerRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM players WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
