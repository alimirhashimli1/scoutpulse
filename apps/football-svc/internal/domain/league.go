package domain

import "time"

type League struct {
	ID        string    `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Country   string    `db:"country" json:"country"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
