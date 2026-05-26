package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/identity-svc/internal/model"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByIdentifier(ctx context.Context, identifier string) (*model.User, error)
}

type PostgresUserRepository struct {
	db *sqlx.DB
}

func NewPostgresUserRepository(db *sqlx.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) Create(ctx context.Context, user *model.User) error {
	query := `INSERT INTO users (id, username, email, password_hash, role, managed_team_ids) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, query, user.ID, user.Username, user.Email, user.PasswordHash, user.Role, user.ManagedTeamIDs)
	return err
}

func (r *PostgresUserRepository) GetByIdentifier(ctx context.Context, identifier string) (*model.User, error) {
	var user model.User
	query := `SELECT id, username, email, password_hash, role, managed_team_ids FROM users WHERE email = $1 OR username = $1`
	err := r.db.GetContext(ctx, &user, query, identifier)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
