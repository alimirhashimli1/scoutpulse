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
	ID           string    `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Role         Role      `json:"role" db:"role"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
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
