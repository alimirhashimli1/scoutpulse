// Package events is the asynchronous spine between services.
//
// It exists so that new apps can be added without editing the ones that
// already exist. football-svc publishes that a player was transferred; a
// notification app, a transfer feed, a search indexer and an analytics app can
// each subscribe, and football-svc never learns they are there. The
// alternative -- synchronous calls from the core service out to each consumer
// -- makes the core service's dependency list grow with every app added.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Subjects published by the football service.
//
// The naming is <entity>.<past-tense-fact>. Events describe something that has
// already happened, so a consumer can never reject one -- it can only react.
const (
	SubjectPlayerTransferred  = "football.player.transferred"
	SubjectPlayerValueChanged = "football.player.value_changed"
	SubjectPlayerCreated      = "football.player.created"
	SubjectCoachAppointed     = "football.coach.appointed"
	SubjectTeamCreated        = "football.team.created"
	SubjectTeamDeleted        = "football.team.deleted"
)

// AllFootballSubjects is the wildcard a consumer can use to receive everything
// the football service emits.
const AllFootballSubjects = "football.>"

// Envelope wraps every published payload with the metadata a consumer needs to
// deduplicate, order, and trace.
type Envelope struct {
	// ID uniquely identifies this event. Delivery is at-least-once, so
	// consumers must treat a repeated ID as a duplicate.
	ID      string `json:"id"`
	Subject string `json:"subject"`
	// OccurredAt is when the fact became true, not when it was published.
	OccurredAt time.Time `json:"occurred_at"`
	// RequestID ties the event back to the HTTP request that caused it.
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

// Decode unmarshals the payload into v.
func (e Envelope) Decode(v any) error {
	if err := json.Unmarshal(e.Payload, v); err != nil {
		return fmt.Errorf("decoding %s payload: %w", e.Subject, err)
	}
	return nil
}

// Publisher emits domain events.
//
// Publishing must never fail a write that has already been committed: the
// database is the source of truth, and a dropped event is a delivery problem,
// not a data-integrity one. Implementations log and swallow transport
// failures rather than propagating them into the caller's transaction.
type Publisher interface {
	Publish(ctx context.Context, subject string, payload any) error
	Close() error
}

// Handler processes a delivered event.
type Handler func(ctx context.Context, e Envelope) error

// Subscriber receives domain events. A new app implements its integration by
// subscribing here rather than by being called.
type Subscriber interface {
	// Subscribe registers h for subject. queueGroup makes delivery
	// competing rather than broadcast: every replica of one app shares a
	// group so exactly one of them handles each event.
	Subscribe(ctx context.Context, subject, queueGroup string, h Handler) error
	Close() error
}

// --- payloads ----------------------------------------------------------

// PlayerTransferred is emitted when a move is recorded.
type PlayerTransferred struct {
	TransferID   string    `json:"transfer_id"`
	PlayerID     string    `json:"player_id"`
	PlayerName   string    `json:"player_name"`
	FromTeamID   *string   `json:"from_team_id"`
	ToTeamID     *string   `json:"to_team_id"`
	TransferType string    `json:"transfer_type"`
	FeeMinor     *int64    `json:"fee_minor"`
	Currency     string    `json:"currency"`
	TransferDate time.Time `json:"transfer_date"`
}

// PlayerValueChanged is emitted when a valuation is recorded.
type PlayerValueChanged struct {
	PlayerID      string    `json:"player_id"`
	PreviousMinor int64     `json:"previous_minor"`
	NewMinor      int64     `json:"new_minor"`
	Currency      string    `json:"currency"`
	ValuedOn      time.Time `json:"valued_on"`
}

// PlayerCreated is emitted when a player enters the database.
type PlayerCreated struct {
	PlayerID string  `json:"player_id"`
	Name     string  `json:"name"`
	TeamID   *string `json:"team_id"`
	Position string  `json:"position"`
}

// CoachAppointed is emitted when a coach spell opens.
type CoachAppointed struct {
	SpellID   string    `json:"spell_id"`
	CoachID   string    `json:"coach_id"`
	CoachName string    `json:"coach_name"`
	TeamID    *string   `json:"team_id"`
	Role      string    `json:"role"`
	StartDate time.Time `json:"start_date"`
}

// TeamCreated is emitted when a club is added.
type TeamCreated struct {
	TeamID   string  `json:"team_id"`
	Name     string  `json:"name"`
	LeagueID *string `json:"league_id"`
}

// TeamDeleted is emitted when a club is removed. Players are not deleted with
// it; they become free agents.
type TeamDeleted struct {
	TeamID string `json:"team_id"`
}
