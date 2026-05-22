package model

import "github.com/lib/pq"

type Role string

const (
	AdminRole  Role = "admin"
	EditorRole Role = "editor"
	UserRole   Role = "user"
)

type User struct {
	ID             string         `json:"id" db:"id"`
	Username       string         `json:"username" db:"username"`
	Email          string         `json:"email" db:"email"`
	PasswordHash   string         `json:"-" db:"password_hash"`
	Role           Role           `json:"role" db:"role"`
	ManagedTeamIDs pq.StringArray `json:"managed_team_ids" db:"managed_team_ids"`
}
