package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

type LeagueRepository interface {
	GetByID(ctx context.Context, id string) (*domain.League, error)
	List(ctx context.Context, page domain.Page) ([]domain.League, error)
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

const leagueColumns = `id, name, country, tier, competition_type, created_at`

func (r *postgresLeagueRepository) GetByID(ctx context.Context, id string) (*domain.League, error) {
	var league domain.League
	query := `SELECT ` + leagueColumns + ` FROM leagues WHERE id = $1`
	if err := r.db.GetContext(ctx, &league, query, id); err != nil {
		return nil, translate("league", err)
	}
	return &league, nil
}

func (r *postgresLeagueRepository) List(ctx context.Context, page domain.Page) ([]domain.League, error) {
	var leagues []domain.League
	// Ordering by a unique-enough key keeps offset paging stable across calls.
	query := `SELECT ` + leagueColumns + ` FROM leagues ORDER BY name, id LIMIT $1 OFFSET $2`
	if err := r.db.SelectContext(ctx, &leagues, query, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("league", err)
	}
	return leagues, nil
}

func (r *postgresLeagueRepository) Create(ctx context.Context, league *domain.League) error {
	query := `INSERT INTO leagues (name, country, tier, competition_type)
	          VALUES ($1, $2, $3, $4) RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query,
		league.Name, league.Country, league.Tier, string(league.CompetitionType)).
		Scan(&league.ID, &league.CreatedAt)
	return translate("league", err)
}

func (r *postgresLeagueRepository) Update(ctx context.Context, league *domain.League) error {
	query := `UPDATE leagues SET name = $1, country = $2, tier = $3, competition_type = $4 WHERE id = $5`
	res, err := r.db.ExecContext(ctx, query,
		league.Name, league.Country, league.Tier, string(league.CompetitionType), league.ID)
	return affected("league", res, err)
}

func (r *postgresLeagueRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM leagues WHERE id = $1`, id)
	return affected("league", res, err)
}
