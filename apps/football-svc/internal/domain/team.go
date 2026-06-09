package domain

import "time"

type Team struct {
	ID        string    `db:"id" json:"id"`
	LeagueID  string    `db:"league_id" json:"league_id"`
	Name      string    `db:"name" json:"name"`
	FanBadge  *string   `db:"fan_badge" json:"fan_badge"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
