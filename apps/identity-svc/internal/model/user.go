package model

import "time"

type Role string

const (
	AdminRole  Role = "admin"
	EditorRole Role = "editor"
	UserRole   Role = "user"
)

// ValidRole reports whether r is a role the system recognises.
func ValidRole(r Role) bool {
	switch r {
	case AdminRole, EditorRole, UserRole:
		return true
	default:
		return false
	}
}

// User is an account.
//
// It carries no team grants: which clubs an editor may modify is football-
// domain data owned by the football service. Keeping them here meant copying
// them into every token, where a revocation could not take effect until the
// token expired.
type User struct {
	ID           string `json:"id" db:"id"`
	Username     string `json:"username" db:"username"`
	Email        string `json:"email" db:"email"`
	PasswordHash string `json:"-" db:"password_hash"`
	Role         Role   `json:"role" db:"role"`
	// EmailVerified is false until the address is proven. Accounts created
	// through an external provider are verified on arrival, because the
	// provider has already done it.
	EmailVerified bool      `json:"email_verified" db:"email_verified"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// Identity links a local account to an account at an external provider.
//
// Accounts are keyed on ProviderUserID, never on Email: an address at a
// provider can be changed or reassigned to somebody else, while the provider's
// subject id is stable for the life of that account.
type Identity struct {
	ID             string     `db:"id" json:"id"`
	UserID         string     `db:"user_id" json:"user_id"`
	Provider       string     `db:"provider" json:"provider"`
	ProviderUserID string     `db:"provider_user_id" json:"-"`
	Email          *string    `db:"email" json:"email,omitempty"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
	LastLoginAt    *time.Time `db:"last_login_at" json:"last_login_at,omitempty"`
}

// RefreshToken is a server-side session record.
//
// The plaintext token is returned to the client once and never stored: only
// its hash lives here, so reading this table does not yield working sessions.
type RefreshToken struct {
	ID         string     `db:"id"`
	UserID     string     `db:"user_id"`
	TokenHash  []byte     `db:"token_hash"`
	IssuedAt   time.Time  `db:"issued_at"`
	ExpiresAt  time.Time  `db:"expires_at"`
	RevokedAt  *time.Time `db:"revoked_at"`
	ReplacedBy *string    `db:"replaced_by"`
	UserAgent  *string    `db:"user_agent"`
}

// Active reports whether the token may still be exchanged.
func (t RefreshToken) Active(now time.Time) bool {
	return t.RevokedAt == nil && now.Before(t.ExpiresAt)
}
