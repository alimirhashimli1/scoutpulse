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
	// UpdatePassword replaces the stored hash. Callers must have verified the
	// current password first; this does not check it.
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	// List returns accounts, optionally narrowed by a substring of the
	// username or email. Administrative: there is no way to see accounts
	// otherwise.
	List(ctx context.Context, query string, limit, offset int) ([]model.User, error)
	// MarkEmailVerified records that the address was proven. Idempotent: a
	// second click on a link that already worked is not an error.
	MarkEmailVerified(ctx context.Context, userID string) error
	// Delete removes an account. Its sessions go with it through
	// ON DELETE CASCADE on refresh_tokens.
	Delete(ctx context.Context, userID string) error
}

type PostgresUserRepository struct {
	db *sqlx.DB
}

func NewPostgresUserRepository(db *sqlx.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

// password_hash is nullable since migration 000003: an account created through
// a provider has none. COALESCE keeps the Go field a plain string, where empty
// means "no password set" — which is what the password-change path checks for.
const userColumns = `id, username, email, COALESCE(password_hash, '') AS password_hash, role, email_verified, created_at`

func (r *PostgresUserRepository) Create(ctx context.Context, user *model.User) error {
	query := `INSERT INTO users (id, username, email, password_hash, role, email_verified)
	          VALUES ($1, $2, $3, $4, $5, $6) RETURNING created_at`
	err := r.db.QueryRowContext(ctx, query,
		user.ID, user.Username, user.Email, user.PasswordHash, user.Role, user.EmailVerified).
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

func (r *PostgresUserRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	query := `UPDATE users SET password_hash = $1 WHERE id = $2`
	res, err := r.db.ExecContext(ctx, query, passwordHash, userID)
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

func (r *PostgresUserRepository) List(ctx context.Context, query string, limit, offset int) ([]model.User, error) {
	var (
		users []model.User
		err   error
	)

	// Ordered by (username, id) so paging is stable. ILIKE rather than the
	// full-text machinery the football service uses: this is an administrative
	// lookup over a small table, not a search box, and a substring match is
	// what an administrator hunting for "ali" actually wants.
	if query != "" {
		sql := `SELECT ` + userColumns + ` FROM users
		         WHERE username ILIKE '%' || $1 || '%' OR email ILIKE '%' || $1 || '%'
		         ORDER BY username, id LIMIT $2 OFFSET $3`
		err = r.db.SelectContext(ctx, &users, sql, query, limit, offset)
	} else {
		sql := `SELECT ` + userColumns + ` FROM users
		         ORDER BY username, id LIMIT $1 OFFSET $2`
		err = r.db.SelectContext(ctx, &users, sql, limit, offset)
	}

	if err != nil {
		return nil, translateUserErr(err)
	}
	return users, nil
}

func (r *PostgresUserRepository) Delete(ctx context.Context, userID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
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
		case "22P02": // invalid_text_representation
			// Every id here is a UUID, so a malformed one reaches Postgres and
			// returns a driver error. Without this case it degrades to a 500
			// for what is plainly a bad request — the same gap N22 closed in
			// the football service.
			return apperr.Wrap(apperr.KindInvalid, "malformed identifier", err)
		}
	}
	return apperr.Internal(err)
}

func (r *PostgresUserRepository) MarkEmailVerified(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET email_verified = TRUE, updated_at = NOW() WHERE id = $1`, userID)
	if err != nil {
		return apperr.Wrap(apperr.KindInternal, "could not record the verification", err)
	}
	return nil
}
