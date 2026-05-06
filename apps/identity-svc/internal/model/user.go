package model

type Role string

const (
	Admin  Role = "ADMIN"
	Editor Role = "EDITOR"
	UserRole   Role = "USER"
)

type User struct {
	ID           string `json:"id" db:"id"`
	Username     string `json:"username" db:"username"`
	Email        string `json:"email" db:"email"`
	PasswordHash string `json:"-" db:"password_hash"`
	Role         Role   `json:"role" db:"role"`
}
