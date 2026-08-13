package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
)

// IdentityRepository stores the link between a local account and an account at
// an external provider.
type IdentityRepository interface {
	// GetByProviderAccount finds the local account a provider account is
	// linked to. This is the primary lookup on every provider sign-in.
	GetByProviderAccount(ctx context.Context, provider, providerUserID string) (*model.Identity, error)
	// ListForUser returns the providers an account has linked, so a settings
	// page can show them and offer to unlink.
	ListForUser(ctx context.Context, userID string) ([]model.Identity, error)
	// Link attaches a provider account to a local one.
	Link(ctx context.Context, identity *model.Identity) error
	// Unlink detaches one.
	Unlink(ctx context.Context, userID, provider string) error
	// TouchLogin records that this identity was just used.
	TouchLogin(ctx context.Context, id string) error
}

type PostgresIdentityRepository struct {
	db *sqlx.DB
}

func NewPostgresIdentityRepository(db *sqlx.DB) *PostgresIdentityRepository {
	return &PostgresIdentityRepository{db: db}
}

const identityColumns = `id, user_id, provider, provider_user_id, email, created_at, last_login_at`

func (r *PostgresIdentityRepository) GetByProviderAccount(ctx context.Context, provider, providerUserID string) (*model.Identity, error) {
	var identity model.Identity
	query := `SELECT ` + identityColumns + ` FROM user_identities
	           WHERE provider = $1 AND provider_user_id = $2`
	if err := r.db.GetContext(ctx, &identity, query, provider, providerUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("identity not found")
		}
		return nil, translateUserErr(err)
	}
	return &identity, nil
}

func (r *PostgresIdentityRepository) ListForUser(ctx context.Context, userID string) ([]model.Identity, error) {
	var identities []model.Identity
	query := `SELECT ` + identityColumns + ` FROM user_identities
	           WHERE user_id = $1 ORDER BY provider`
	if err := r.db.SelectContext(ctx, &identities, query, userID); err != nil {
		return nil, translateUserErr(err)
	}
	return identities, nil
}

func (r *PostgresIdentityRepository) Link(ctx context.Context, i *model.Identity) error {
	query := `INSERT INTO user_identities (user_id, provider, provider_user_id, email, last_login_at)
	          VALUES ($1, $2, $3, $4, NOW())
	          RETURNING id, created_at, last_login_at`
	err := r.db.QueryRowContext(ctx, query, i.UserID, i.Provider, i.ProviderUserID, i.Email).
		Scan(&i.ID, &i.CreatedAt, &i.LastLoginAt)
	if err != nil {
		// A unique violation here means the provider account is already linked
		// to a different local account, or this account already has this
		// provider. Either way it is a conflict, not a server fault.
		return translateIdentityErr(err)
	}
	return nil
}

func (r *PostgresIdentityRepository) Unlink(ctx context.Context, userID, provider string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM user_identities WHERE user_id = $1 AND provider = $2`, userID, provider)
	if err != nil {
		return translateUserErr(err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return apperr.Internal(err)
	}
	if rows == 0 {
		return apperr.NotFound("that provider is not linked to this account")
	}
	return nil
}

func (r *PostgresIdentityRepository) TouchLogin(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE user_identities SET last_login_at = NOW() WHERE id = $1`, id)
	return translateUserErr(err)
}

func translateIdentityErr(err error) error {
	if err == nil {
		return nil
	}
	translated := translateUserErr(err)
	if apperr.KindOf(translated) == apperr.KindConflict {
		return apperr.Wrap(apperr.KindConflict,
			"that provider account is already linked to an account", err)
	}
	return translated
}

// --- one-time login codes ---------------------------------------------

// LoginCodeRepository issues the short-lived code that carries a completed
// provider sign-in from the backend callback to the browser app.
//
// The alternative is putting tokens in the redirect URL, which writes a
// refresh token into browser history, server access logs, and any Referer
// header the next page sends.
type LoginCodeRepository interface {
	// Issue creates a code for a user. The plaintext is returned once and
	// never stored.
	Issue(ctx context.Context, userID string, ttl time.Duration) (string, error)
	// Redeem exchanges a code for its user id. A code works exactly once;
	// presenting a used or expired one fails.
	Redeem(ctx context.Context, code string) (string, error)
	// DeleteExpired prunes the table.
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

type PostgresLoginCodeRepository struct {
	db *sqlx.DB
}

func NewPostgresLoginCodeRepository(db *sqlx.DB) *PostgresLoginCodeRepository {
	return &PostgresLoginCodeRepository{db: db}
}

// HashLoginCode derives the stored form. As with refresh tokens, the code is
// high-entropy random rather than a password, so a plain SHA-256 is right: a
// slow KDF would add latency without adding resistance to anything.
func HashLoginCode(code string) []byte {
	sum := sha256.Sum256([]byte(code))
	return sum[:]
}

func (r *PostgresLoginCodeRepository) Issue(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	// The same generator refresh tokens use: 256 bits of randomness, base64url
	// encoded. Nothing about a login code needs to differ.
	code, err := auth.NewRefreshToken()
	if err != nil {
		return "", apperr.Internal(err)
	}

	query := `INSERT INTO oauth_login_codes (code_hash, user_id, expires_at) VALUES ($1, $2, $3)`
	if _, err := r.db.ExecContext(ctx, query, HashLoginCode(code), userID, time.Now().Add(ttl)); err != nil {
		return "", apperr.Wrap(apperr.KindInternal, "could not start the sign-in", err)
	}
	return code, nil
}

func (r *PostgresLoginCodeRepository) Redeem(ctx context.Context, code string) (string, error) {
	// Marking it used in the same statement that reads it makes redemption
	// atomic: two concurrent exchanges of one code cannot both succeed,
	// because only one UPDATE can match the used_at IS NULL predicate.
	query := `UPDATE oauth_login_codes
	             SET used_at = NOW()
	           WHERE code_hash = $1 AND used_at IS NULL AND expires_at > NOW()
	       RETURNING user_id`

	var userID string
	err := r.db.QueryRowContext(ctx, query, HashLoginCode(code)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		// Unknown, already used, or expired — all reported identically, since
		// distinguishing them would tell a guesser which codes had existed.
		return "", apperr.Unauthorized("invalid or expired sign-in code")
	}
	if err != nil {
		return "", apperr.Wrap(apperr.KindInternal, "could not complete the sign-in", err)
	}
	return userID, nil
}

func (r *PostgresLoginCodeRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM oauth_login_codes WHERE expires_at < $1`, before)
	if err != nil {
		return 0, apperr.Wrap(apperr.KindInternal, "could not prune sign-in codes", err)
	}
	return res.RowsAffected()
}
