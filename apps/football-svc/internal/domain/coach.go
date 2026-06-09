package domain

import "time"

type Coach struct {
	ID            string     `db:"id" json:"id"`
	TeamID        string     `db:"team_id" json:"team_id"`
	Name          string     `db:"name" json:"name"`
	ContractUntil *time.Time `db:"contract_until" json:"contract_until"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
}
