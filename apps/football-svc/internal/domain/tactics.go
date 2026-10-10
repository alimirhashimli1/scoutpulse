package domain

import (
	"encoding/json"
	"time"
)

// TacticBoard is one member's animated tactics board: a drill, a shape, an
// analysis of a match.
//
// Boards are unlisted rather than private. Anyone holding the link can watch
// one -- the players and coaches it is made for have no accounts -- but they
// appear nowhere a signed-out visitor can browse. The slug is the address, and
// it is random so that holding one link reveals nothing about any other.
type TacticBoard struct {
	// ID is internal. The slug is the identifier the API speaks in, so there
	// is one address for a board rather than two that could be confused.
	ID      string `db:"id" json:"-"`
	Slug    string `db:"slug" json:"slug"`
	OwnerID string `db:"owner_id" json:"owner_id"`
	// OwnerName is captured from the token when the board is written, because
	// this service cannot resolve a user id to a name.
	OwnerName   string `db:"owner_name" json:"owner_name"`
	Title       string `db:"title" json:"title"`
	Description string `db:"description" json:"description"`
	// Content is the board document. Its shape belongs to the editor; this
	// service only checks that it is a JSON object of bounded size.
	Content   json.RawMessage `db:"content" json:"content"`
	CreatedAt time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt time.Time       `db:"updated_at" json:"updated_at"`
}

// TacticSummary is a board without its document, for lists.
//
// A list of fifty boards should not carry fifty animations nobody is watching
// yet. Steps is computed in the query, so the list can say how long each one
// is without shipping it.
type TacticSummary struct {
	Slug        string    `db:"slug" json:"slug"`
	OwnerID     string    `db:"owner_id" json:"owner_id"`
	OwnerName   string    `db:"owner_name" json:"owner_name"`
	Title       string    `db:"title" json:"title"`
	Description string    `db:"description" json:"description"`
	Steps       int       `db:"steps" json:"steps"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}
