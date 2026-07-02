package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

type TeamRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Team, error)
	ListByLeague(ctx context.Context, leagueID string) ([]domain.Team, error)
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

func (r *postgresTeamRepository) GetByID(ctx context.Context, id string) (*domain.Team, error) {
	var team domain.Team
	query := `SELECT id, league_id, name, fan_badge_url, created_at FROM teams WHERE id = $1`
	err := r.db.GetContext(ctx, &team, query, id)
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (r *postgresTeamRepository) ListByLeague(ctx context.Context, leagueID string) ([]domain.Team, error) {
	var teams []domain.Team
	query := `SELECT id, league_id, name, fan_badge_url, created_at FROM teams WHERE league_id = $1 ORDER BY name`
	err := r.db.SelectContext(ctx, &teams, query, leagueID)
	if err != nil {
		return nil, err
	}
	return teams, nil
}

func (r *postgresTeamRepository) Create(ctx context.Context, team *domain.Team) error {
	query := `INSERT INTO teams (league_id, name, fan_badge_url) VALUES ($1, $2, $3) RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, team.LeagueID, team.Name, team.FanBadgeURL).Scan(&team.ID, &team.CreatedAt)
	return err
}

func (r *postgresTeamRepository) Update(ctx context.Context, team *domain.Team) error {
	query := `UPDATE teams SET league_id = $1, name = $2, fan_badge_url = $3 WHERE id = $4`
	_, err := r.db.ExecContext(ctx, query, team.LeagueID, team.Name, team.FanBadgeURL, team.ID)
	return err
}

func (r *postgresTeamRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM teams WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
