package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

type CoachRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Coach, error)
	GetByTeam(ctx context.Context, teamID string) (*domain.Coach, error)
	Create(ctx context.Context, coach *domain.Coach) error
	Update(ctx context.Context, coach *domain.Coach) error
	Delete(ctx context.Context, id string) error
}

type postgresCoachRepository struct {
	db *sqlx.DB
}

func NewPostgresCoachRepository(db *sqlx.DB) CoachRepository {
	return &postgresCoachRepository{db: db}
}

const coachColumns = `id, team_id, name, contract_until, created_at`

func (r *postgresCoachRepository) GetByID(ctx context.Context, id string) (*domain.Coach, error) {
	var coach domain.Coach
	query := `SELECT ` + coachColumns + ` FROM coaches WHERE id = $1`
	if err := r.db.GetContext(ctx, &coach, query, id); err != nil {
		return nil, translate("coach", err)
	}
	return &coach, nil
}

func (r *postgresCoachRepository) GetByTeam(ctx context.Context, teamID string) (*domain.Coach, error) {
	var coach domain.Coach
	query := `SELECT ` + coachColumns + ` FROM coaches WHERE team_id = $1`
	if err := r.db.GetContext(ctx, &coach, query, teamID); err != nil {
		return nil, translate("coach", err)
	}
	return &coach, nil
}

func (r *postgresCoachRepository) Create(ctx context.Context, coach *domain.Coach) error {
	query := `INSERT INTO coaches (team_id, name, contract_until) VALUES ($1, $2, $3) RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, coach.TeamID, coach.Name, coach.ContractUntil).
		Scan(&coach.ID, &coach.CreatedAt)
	return translate("coach", err)
}

func (r *postgresCoachRepository) Update(ctx context.Context, coach *domain.Coach) error {
	query := `UPDATE coaches SET team_id = $1, name = $2, contract_until = $3 WHERE id = $4`
	res, err := r.db.ExecContext(ctx, query, coach.TeamID, coach.Name, coach.ContractUntil, coach.ID)
	return affected("coach", res, err)
}

func (r *postgresCoachRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM coaches WHERE id = $1`, id)
	return affected("coach", res, err)
}
