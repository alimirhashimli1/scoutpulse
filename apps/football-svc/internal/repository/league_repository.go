package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

type LeagueRepository interface {
	GetByID(ctx context.Context, id string) (*domain.League, error)
	List(ctx context.Context) ([]domain.League, error)
	Create(ctx context.Context, league *domain.League) error
	Update(ctx context.Context, league *domain.League) error
	Delete(ctx context.Context, id string) error
}

type postgresLeagueRepository struct {
	db *sqlx.DB
}

func NewPostgresLeagueRepository(db *sqlx.DB) LeagueRepository {
	return &postgresLeagueRepository{db: db}
}

func (r *postgresLeagueRepository) GetByID(ctx context.Context, id string) (*domain.League, error) {
	var league domain.League
	query := `SELECT id, name, country, created_at FROM leagues WHERE id = $1`
	err := r.db.GetContext(ctx, &league, query, id)
	if err != nil {
		return nil, err
	}
	return &league, nil
}

func (r *postgresLeagueRepository) List(ctx context.Context) ([]domain.League, error) {
	var leagues []domain.League
	query := `SELECT id, name, country, created_at FROM leagues ORDER BY name`
	err := r.db.SelectContext(ctx, &leagues, query)
	if err != nil {
		return nil, err
	}
	return leagues, nil
}

func (r *postgresLeagueRepository) Create(ctx context.Context, league *domain.League) error {
	query := `INSERT INTO leagues (name, country) VALUES ($1, $2) RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, league.Name, league.Country).Scan(&league.ID, &league.CreatedAt)
	return err
}

func (r *postgresLeagueRepository) Update(ctx context.Context, league *domain.League) error {
	query := `UPDATE leagues SET name = $1, country = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, league.Name, league.Country, league.ID)
	return err
}

func (r *postgresLeagueRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM leagues WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
