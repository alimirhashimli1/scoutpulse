package domain

import "time"

// League represents a football league entity.
type League struct {
	ID        string    `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Country   string    `db:"country" json:"country"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// Team represents a football club/team entity.
type Team struct {
	ID          string    `db:"id" json:"id"`
	LeagueID    *string   `db:"league_id" json:"league_id"` // Nullable to support deletion of a league (free agents/free teams)
	Name        string    `db:"name" json:"name"`
	FanBadgeURL *string   `db:"fan_badge_url" json:"fan_badge_url"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// Coach represents a team's head coach entity.
type Coach struct {
	ID            string     `db:"id" json:"id"`
	TeamID        *string    `db:"team_id" json:"team_id"` // Nullable to support free agency/fired coach
	Name          string     `db:"name" json:"name"`
	ContractUntil *time.Time `db:"contract_until" json:"contract_until"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
}

// Player represents a football player entity.
type Player struct {
	ID            string     `db:"id" json:"id"`
	TeamID        *string    `db:"team_id" json:"team_id"` // Nullable to support free agency
	Name          string     `db:"name" json:"name"`
	Position      string     `db:"position" json:"position"`
	MarketValue   float64    `db:"market_value" json:"market_value"`
	ContractUntil *time.Time `db:"contract_until" json:"contract_until"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
}
