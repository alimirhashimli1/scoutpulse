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

const coachColumns = `id, team_id, name, first_name, last_name, date_of_birth, nationality,
	contract_until, created_at`

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
	query := `INSERT INTO coaches
		(team_id, name, first_name, last_name, date_of_birth, nationality, contract_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query,
		coach.TeamID, coach.Name, coach.FirstName, coach.LastName,
		coach.DateOfBirth, coach.Nationality, coach.ContractUntil,
	).Scan(&coach.ID, &coach.CreatedAt)
	return translate("coach", err)
}

// Update writes the coach's descriptive fields. team_id is absent: the current
// appointment is derived state owned by the coach-spell flow.
func (r *postgresCoachRepository) Update(ctx context.Context, coach *domain.Coach) error {
	query := `UPDATE coaches SET name = $1, first_name = $2, last_name = $3,
		date_of_birth = $4, nationality = $5, contract_until = $6 WHERE id = $7`
	res, err := r.db.ExecContext(ctx, query,
		coach.Name, coach.FirstName, coach.LastName,
		coach.DateOfBirth, coach.Nationality, coach.ContractUntil, coach.ID,
	)
	return affected("coach", res, err)
}

func (r *postgresCoachRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM coaches WHERE id = $1`, id)
	return affected("coach", res, err)
}
