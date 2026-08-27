package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
)

// EmailVerificationRepository issues and redeems the links that prove someone
// controls the address they registered with.
//
// The shape deliberately mirrors PostgresLoginCodeRepository: a high-entropy
// token, stored only as a SHA-256 hash, redeemed atomically and exactly once.
// A verification link sits in an inbox indefinitely, so single use matters as
// much here as it does for a sign-in code.
type EmailVerificationRepository interface {
	// Issue creates a token for a user, invalidating any outstanding ones.
	Issue(ctx context.Context, userID string, ttl time.Duration) (string, error)
	// Redeem consumes a token and returns the user it belongs to.
	Redeem(ctx context.Context, token string) (string, error)
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

type PostgresEmailVerificationRepository struct {
	db *sqlx.DB
}

func NewEmailVerificationRepository(db *sqlx.DB) *PostgresEmailVerificationRepository {
	return &PostgresEmailVerificationRepository{db: db}
}

// HashVerificationToken derives the stored form. A plain SHA-256 is correct
// for the same reason it is for refresh tokens: the input is 256 bits of
// randomness, not a guessable secret, so a slow KDF would add latency without
// adding resistance to anything.
func HashVerificationToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (r *PostgresEmailVerificationRepository) Issue(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	token, err := auth.NewRefreshToken()
	if err != nil {
		return "", apperr.Internal(err)
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", apperr.Wrap(apperr.KindInternal, "could not start verification", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Outstanding tokens are consumed rather than deleted, so a resend
	// invalidates the previous link. Without this, every email ever sent stays
	// valid until it expires -- and "resend" is exactly what someone does when
	// they suspect the first message went somewhere it should not have.
	if _, err := tx.ExecContext(ctx,
		`UPDATE email_verification_tokens SET consumed_at = NOW()
		  WHERE user_id = $1 AND consumed_at IS NULL`, userID); err != nil {
		return "", apperr.Wrap(apperr.KindInternal, "could not start verification", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO email_verification_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		HashVerificationToken(token), userID, time.Now().Add(ttl)); err != nil {
		return "", apperr.Wrap(apperr.KindInternal, "could not start verification", err)
	}

	if err := tx.Commit(); err != nil {
		return "", apperr.Wrap(apperr.KindInternal, "could not start verification", err)
	}
	return token, nil
}

func (r *PostgresEmailVerificationRepository) Redeem(ctx context.Context, token string) (string, error) {
	// Consumed in the same statement that reads it, so two clicks on the same
	// link cannot both succeed -- only one UPDATE can match consumed_at IS NULL.
	// Email clients that pre-fetch links make this a real case, not a
	// theoretical one.
	query := `UPDATE email_verification_tokens
	             SET consumed_at = NOW()
	           WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > NOW()
	       RETURNING user_id`

	var userID string
	err := r.db.QueryRowContext(ctx, query, HashVerificationToken(token)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		// Unknown, already used and expired are reported identically. The
		// distinction would tell someone probing which tokens had once been
		// real, and the remedy is the same in every case: request a new link.
		return "", apperr.Invalid("this link is invalid or has expired. Request a new one.")
	}
	if err != nil {
		return "", apperr.Wrap(apperr.KindInternal, "could not verify the address", err)
	}
	return userID, nil
}

func (r *PostgresEmailVerificationRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM email_verification_tokens WHERE expires_at < $1`, before)
	if err != nil {
		return 0, apperr.Wrap(apperr.KindInternal, "could not prune verification tokens", err)
	}
	return res.RowsAffected()
}
