package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

type TeamRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Team, error)
	ListByLeague(ctx context.Context, leagueID string, page domain.Page) ([]domain.Team, error)
	Create(ctx context.Context, team *domain.Team) error
	Update(ctx context.Context, team *domain.Team) error
	Delete(ctx context.Context, id string) error
}

type postgresTeamRepository struct {
	db *sqlx.DB
}

func NewPostgresTeamRepository(db *sqlx.DB) TeamRepository {
	return &postgresTeamRepository{db: db}
}

const teamColumns = `id, league_id, name, fan_badge_url, created_at`

func (r *postgresTeamRepository) GetByID(ctx context.Context, id string) (*domain.Team, error) {
	var team domain.Team
	query := `SELECT ` + teamColumns + ` FROM teams WHERE id = $1`
	if err := r.db.GetContext(ctx, &team, query, id); err != nil {
		return nil, translate("team", err)
	}
	return &team, nil
}

func (r *postgresTeamRepository) ListByLeague(ctx context.Context, leagueID string, page domain.Page) ([]domain.Team, error) {
	var teams []domain.Team
	query := `SELECT ` + teamColumns + ` FROM teams WHERE league_id = $1 ORDER BY name, id LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &teams, query, leagueID, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("team", err)
	}
	return teams, nil
}

func (r *postgresTeamRepository) Create(ctx context.Context, team *domain.Team) error {
	query := `INSERT INTO teams (league_id, name, fan_badge_url) VALUES ($1, $2, $3) RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, team.LeagueID, team.Name, team.FanBadgeURL).
		Scan(&team.ID, &team.CreatedAt)
	return translate("team", err)
}

func (r *postgresTeamRepository) Update(ctx context.Context, team *domain.Team) error {
	query := `UPDATE teams SET league_id = $1, name = $2, fan_badge_url = $3 WHERE id = $4`
	res, err := r.db.ExecContext(ctx, query, team.LeagueID, team.Name, team.FanBadgeURL, team.ID)
	return affected("team", res, err)
}

func (r *postgresTeamRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM teams WHERE id = $1`, id)
	return affected("team", res, err)
}
