package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

// TeamSeasonRepository records which competitions a club contested in a season.
//
// teams.league_id is the club's current primary competition and answers "where
// do they play now". This table answers "what did they play in, and when" --
// including the several competitions a club enters in a single season, which a
// single column cannot express.
type TeamSeasonRepository interface {
	// ListByTeam returns a club's entries, newest season first.
	ListByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.TeamSeason, error)
	// ListBySeason returns every club entered in a season, optionally narrowed
	// to one competition -- which is what a "who was in this league that year"
	// page needs.
	ListBySeason(ctx context.Context, seasonID string, leagueID *string, page domain.Page) ([]domain.TeamSeason, error)
	// Enter records a club in a competition for a season. Re-entering the same
	// combination is a no-op rather than a conflict.
	Enter(ctx context.Context, entry *domain.TeamSeason) error
	Withdraw(ctx context.Context, id string) error
}

type postgresTeamSeasonRepository struct {
	db *sqlx.DB
}

func NewPostgresTeamSeasonRepository(db *sqlx.DB) TeamSeasonRepository {
	return &postgresTeamSeasonRepository{db: db}
}

const teamSeasonColumns = `id, team_id, season_id, league_id, created_at`

func (r *postgresTeamSeasonRepository) ListByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.TeamSeason, error) {
	var entries []domain.TeamSeason
	// Joined to seasons so the ordering is chronological rather than by a
	// meaningless uuid; idx_team_seasons_season covers the join.
	query := `SELECT ts.id, ts.team_id, ts.season_id, ts.league_id, ts.created_at
	            FROM team_seasons ts
	            JOIN seasons s ON s.id = ts.season_id
	           WHERE ts.team_id = $1
	           ORDER BY s.start_date DESC, ts.id
	           LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &entries, query, teamID, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("team season", err)
	}
	return entries, nil
}

func (r *postgresTeamSeasonRepository) ListBySeason(ctx context.Context, seasonID string, leagueID *string, page domain.Page) ([]domain.TeamSeason, error) {
	var (
		entries []domain.TeamSeason
		err     error
	)

	if leagueID != nil && *leagueID != "" {
		query := `SELECT ` + teamSeasonColumns + ` FROM team_seasons
		           WHERE season_id = $1 AND league_id = $2
		           ORDER BY id LIMIT $3 OFFSET $4`
		err = r.db.SelectContext(ctx, &entries, query, seasonID, *leagueID, page.FetchLimit(), page.Offset)
	} else {
		query := `SELECT ` + teamSeasonColumns + ` FROM team_seasons
		           WHERE season_id = $1
		           ORDER BY id LIMIT $2 OFFSET $3`
		err = r.db.SelectContext(ctx, &entries, query, seasonID, page.FetchLimit(), page.Offset)
	}

	if err != nil {
		return nil, translate("team season", err)
	}
	return entries, nil
}

func (r *postgresTeamSeasonRepository) Enter(ctx context.Context, e *domain.TeamSeason) error {
	// ON CONFLICT DO UPDATE rather than DO NOTHING: a plain DO NOTHING returns
	// no row, so RETURNING yields sql.ErrNoRows and a re-entry would look like
	// a failure. Setting team_id to itself is a no-op write that still returns
	// the existing row.
	query := `INSERT INTO team_seasons (team_id, season_id, league_id)
	          VALUES ($1, $2, $3)
	          ON CONFLICT (team_id, season_id, league_id)
	          DO UPDATE SET team_id = EXCLUDED.team_id
	          RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, e.TeamID, e.SeasonID, e.LeagueID).
		Scan(&e.ID, &e.CreatedAt)
	return translate("team season", err)
}

func (r *postgresTeamSeasonRepository) Withdraw(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM team_seasons WHERE id = $1`, id)
	return affected("team season", res, err)
}
