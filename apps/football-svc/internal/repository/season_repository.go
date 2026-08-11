package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

type SeasonRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Season, error)
	// Current returns the season containing the given date, if one exists.
	Current(ctx context.Context, on time.Time) (*domain.Season, error)
	// Overlapping returns a season whose dates intersect the given range,
	// ignoring excludeID so a season can be updated without matching itself.
	// A nil result means the range is free.
	Overlapping(ctx context.Context, start, end time.Time, excludeID string) (*domain.Season, error)
	List(ctx context.Context, page domain.Page) ([]domain.Season, error)
	Create(ctx context.Context, season *domain.Season) error
	Update(ctx context.Context, season *domain.Season) error
	Delete(ctx context.Context, id string) error
}

type postgresSeasonRepository struct {
	db *sqlx.DB
}

func NewPostgresSeasonRepository(db *sqlx.DB) SeasonRepository {
	return &postgresSeasonRepository{db: db}
}

const seasonColumns = `id, label, start_date, end_date, created_at`

func (r *postgresSeasonRepository) GetByID(ctx context.Context, id string) (*domain.Season, error) {
	var s domain.Season
	if err := r.db.GetContext(ctx, &s, `SELECT `+seasonColumns+` FROM seasons WHERE id = $1`, id); err != nil {
		return nil, translate("season", err)
	}
	return &s, nil
}

func (r *postgresSeasonRepository) Current(ctx context.Context, on time.Time) (*domain.Season, error) {
	var s domain.Season
	query := `SELECT ` + seasonColumns + ` FROM seasons
	          WHERE start_date <= $1 AND end_date >= $1
	          ORDER BY start_date DESC LIMIT 1`
	if err := r.db.GetContext(ctx, &s, query, on); err != nil {
		return nil, translate("season", err)
	}
	return &s, nil
}

func (r *postgresSeasonRepository) Overlapping(ctx context.Context, start, end time.Time, excludeID string) (*domain.Season, error) {
	var s domain.Season
	// Two ranges overlap when each starts before the other ends. The empty
	// excludeID case is handled by the NULLIF cast rather than by branching on
	// the query text.
	query := `SELECT ` + seasonColumns + ` FROM seasons
	          WHERE start_date <= $2 AND end_date >= $1
	            AND ($3 = '' OR id <> NULLIF($3, '')::uuid)
	          ORDER BY start_date LIMIT 1`
	err := r.db.GetContext(ctx, &s, query, start, end, excludeID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, translate("season", err)
	}
	return &s, nil
}

func (r *postgresSeasonRepository) List(ctx context.Context, page domain.Page) ([]domain.Season, error) {
	var seasons []domain.Season
	query := `SELECT ` + seasonColumns + ` FROM seasons ORDER BY start_date DESC, id LIMIT $1 OFFSET $2`
	if err := r.db.SelectContext(ctx, &seasons, query, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("season", err)
	}
	return seasons, nil
}

func (r *postgresSeasonRepository) Create(ctx context.Context, s *domain.Season) error {
	query := `INSERT INTO seasons (label, start_date, end_date) VALUES ($1, $2, $3)
	          RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, s.Label, s.StartDate, s.EndDate).Scan(&s.ID, &s.CreatedAt)
	return translate("season", err)
}

func (r *postgresSeasonRepository) Update(ctx context.Context, s *domain.Season) error {
	query := `UPDATE seasons SET label = $1, start_date = $2, end_date = $3 WHERE id = $4`
	res, err := r.db.ExecContext(ctx, query, s.Label, s.StartDate, s.EndDate, s.ID)
	return affected("season", res, err)
}

func (r *postgresSeasonRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM seasons WHERE id = $1`, id)
	return affected("season", res, err)
}
