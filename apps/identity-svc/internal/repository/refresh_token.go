package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/libs/platform/apperr"
)

// HashToken derives the stored form of a refresh token.
//
// SHA-256 without a salt is deliberate: unlike a password, a refresh token is
// 256 bits of uniform randomness, so it is not guessable and a slow KDF would
// only add latency to every refresh. The hash exists so that a database leak
// does not hand over live sessions.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, t *model.RefreshToken) error
	GetByToken(ctx context.Context, token string) (*model.RefreshToken, error)
	// Rotate revokes the old token and issues its replacement atomically.
	Rotate(ctx context.Context, oldID string, next *model.RefreshToken) error
	Revoke(ctx context.Context, token string) error
	// RevokeAllForUser ends every session a user holds. Used on refresh-token
	// reuse and on a password or role change.
	RevokeAllForUser(ctx context.Context, userID string) error
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

type PostgresRefreshTokenRepository struct {
	db *sqlx.DB
}

func NewPostgresRefreshTokenRepository(db *sqlx.DB) *PostgresRefreshTokenRepository {
	return &PostgresRefreshTokenRepository{db: db}
}

// The column list, not a secret. gosec's G101 matches on the substring
// "token_hash" and cannot tell a SELECT list from a credential.
//
//nolint:gosec // G101 false positive: this is a SQL column list
const refreshTokenColumns = `id, user_id, token_hash, issued_at, expires_at, revoked_at, replaced_by, user_agent`

func (r *PostgresRefreshTokenRepository) Create(ctx context.Context, t *model.RefreshToken) error {
	query := `INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent)
	          VALUES ($1, $2, $3, $4) RETURNING id, issued_at`
	err := r.db.QueryRowContext(ctx, query, t.UserID, t.TokenHash, t.ExpiresAt, t.UserAgent).
		Scan(&t.ID, &t.IssuedAt)
	if err != nil {
		return apperr.Wrap(apperr.KindInternal, "could not store session", err)
	}
	return nil
}

func (r *PostgresRefreshTokenRepository) GetByToken(ctx context.Context, token string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	query := `SELECT ` + refreshTokenColumns + ` FROM refresh_tokens WHERE token_hash = $1`
	if err := r.db.GetContext(ctx, &t, query, HashToken(token)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.Unauthorized("invalid refresh token")
		}
		return nil, apperr.Wrap(apperr.KindInternal, "could not read session", err)
	}
	return &t, nil
}

func (r *PostgresRefreshTokenRepository) Rotate(ctx context.Context, oldID string, next *model.RefreshToken) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return apperr.Wrap(apperr.KindInternal, "could not rotate session", err)
	}
	defer func() { _ = tx.Rollback() }()

	insert := `INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent)
	           VALUES ($1, $2, $3, $4) RETURNING id, issued_at`
	if err := tx.QueryRowContext(ctx, insert, next.UserID, next.TokenHash, next.ExpiresAt, next.UserAgent).
		Scan(&next.ID, &next.IssuedAt); err != nil {
		return apperr.Wrap(apperr.KindInternal, "could not rotate session", err)
	}

	// Revoking only where revoked_at IS NULL makes concurrent refreshes of
	// the same token safe: exactly one wins, and the loser sees zero rows
	// affected rather than silently issuing a second live session.
	revoke := `UPDATE refresh_tokens SET revoked_at = NOW(), replaced_by = $1
	            WHERE id = $2 AND revoked_at IS NULL`
	res, err := tx.ExecContext(ctx, revoke, next.ID, oldID)
	if err != nil {
		return apperr.Wrap(apperr.KindInternal, "could not rotate session", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return apperr.Wrap(apperr.KindInternal, "could not rotate session", err)
	}
	if rows == 0 {
		return apperr.Unauthorized("invalid refresh token")
	}

	if err := tx.Commit(); err != nil {
		return apperr.Wrap(apperr.KindInternal, "could not rotate session", err)
	}
	return nil
}

func (r *PostgresRefreshTokenRepository) Revoke(ctx context.Context, token string) error {
	query := `UPDATE refresh_tokens SET revoked_at = NOW()
	           WHERE token_hash = $1 AND revoked_at IS NULL`
	if _, err := r.db.ExecContext(ctx, query, HashToken(token)); err != nil {
		return apperr.Wrap(apperr.KindInternal, "could not revoke session", err)
	}
	// Revoking an unknown or already-revoked token is not an error: logging
	// out twice should succeed, and reporting which is which would leak
	// whether a token exists.
	return nil
}

func (r *PostgresRefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	query := `UPDATE refresh_tokens SET revoked_at = NOW()
	           WHERE user_id = $1 AND revoked_at IS NULL`
	if _, err := r.db.ExecContext(ctx, query, userID); err != nil {
		return apperr.Wrap(apperr.KindInternal, "could not revoke sessions", err)
	}
	return nil
}

func (r *PostgresRefreshTokenRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE expires_at < $1`, before)
	if err != nil {
		return 0, apperr.Wrap(apperr.KindInternal, "could not prune sessions", err)
	}
	return res.RowsAffected()
}
