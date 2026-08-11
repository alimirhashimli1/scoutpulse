package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/libs/platform/apperr"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByIdentifier(ctx context.Context, identifier string) (*model.User, error)
	UpdateRole(ctx context.Context, userID string, role model.Role, updatedBy string) error
}

type PostgresUserRepository struct {
	db *sqlx.DB
}

func NewPostgresUserRepository(db *sqlx.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

const userColumns = `id, username, email, password_hash, role, created_at`

func (r *PostgresUserRepository) Create(ctx context.Context, user *model.User) error {
	query := `INSERT INTO users (id, username, email, password_hash, role)
	          VALUES ($1, $2, $3, $4, $5) RETURNING created_at`
	err := r.db.QueryRowContext(ctx, query,
		user.ID, user.Username, user.Email, user.PasswordHash, user.Role).
		Scan(&user.CreatedAt)
	return translateUserErr(err)
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	var user model.User
	if err := r.db.GetContext(ctx, &user, `SELECT `+userColumns+` FROM users WHERE id = $1`, id); err != nil {
		return nil, translateUserErr(err)
	}
	return &user, nil
}

func (r *PostgresUserRepository) GetByIdentifier(ctx context.Context, identifier string) (*model.User, error) {
	var user model.User
	query := `SELECT ` + userColumns + ` FROM users WHERE email = $1 OR username = $1`
	if err := r.db.GetContext(ctx, &user, query, identifier); err != nil {
		return nil, translateUserErr(err)
	}
	return &user, nil
}

func (r *PostgresUserRepository) UpdateRole(ctx context.Context, userID string, role model.Role, updatedBy string) error {
	query := `UPDATE users SET role = $1, role_updated_at = NOW(), role_updated_by = $2 WHERE id = $3`
	res, err := r.db.ExecContext(ctx, query, role, updatedBy, userID)
	if err != nil {
		return translateUserErr(err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return apperr.Internal(err)
	}
	if rows == 0 {
		return apperr.NotFound("user not found")
	}
	return nil
}

// translateUserErr converts a database error into the shared taxonomy. The
// driver message can embed SQL and constraint names, so it is carried as an
// internal cause rather than as a client-facing message.
func translateUserErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.NotFound("user not found")
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch string(pqErr.Code) {
		case "23505": // unique_violation
			return apperr.Wrap(apperr.KindConflict, "username or email is already taken", err)
		case "23514": // check_violation
			return apperr.Wrap(apperr.KindInvalid, "a field has an unsupported value", err)
		}
	}
	return apperr.Internal(err)
}
