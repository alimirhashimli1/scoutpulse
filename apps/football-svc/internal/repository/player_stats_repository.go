package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

type PlayerStatsRepository interface {
	// ListByPlayer returns every season line for a player, newest season
	// first, league before cups within a season.
	ListByPlayer(ctx context.Context, playerID string) ([]domain.PlayerSeasonStat, error)
}

type postgresPlayerStatsRepository struct {
	db *sqlx.DB
}

func NewPostgresPlayerStatsRepository(db *sqlx.DB) PlayerStatsRepository {
	return &postgresPlayerStatsRepository{db: db}
}

// Not paginated: a career is a few dozen lines at most -- one per competition
// per season -- and a page boundary through the middle of a season would split
// the table the page draws.
func (r *postgresPlayerStatsRepository) ListByPlayer(ctx context.Context, playerID string) ([]domain.PlayerSeasonStat, error) {
	stats := []domain.PlayerSeasonStat{}
	query := `SELECT s.id, s.player_id, s.season_id, s.league_id, s.team_id,
	                 s.appearances, s.minutes, s.goals, s.assists, s.yellow_cards, s.red_cards
	            FROM player_season_stats s
	            JOIN seasons se ON se.id = s.season_id
	            JOIN leagues l  ON l.id  = s.league_id
	           WHERE s.player_id = $1
	           ORDER BY se.start_date DESC,
	                    CASE l.competition_type WHEN 'league' THEN 0 WHEN 'international_cup' THEN 1
	                                            WHEN 'domestic_cup' THEN 2 ELSE 3 END,
	                    s.minutes DESC, s.id`
	if err := r.db.SelectContext(ctx, &stats, query, playerID); err != nil {
		return nil, translate("player stats", err)
	}
	return stats, nil
}
