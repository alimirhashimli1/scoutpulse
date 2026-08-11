package repository

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/lib/pq"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
)

// TestTranslate_MalformedUUIDIsInvalidNotInternal pins the fix for N22.
//
// Every id in this schema is a UUID, so "/players/not-a-uuid" reaches Postgres
// and comes back as 22P02. Before this mapping existed it fell through to
// KindInternal, which meant a client typo produced a 500 and an error line in
// the log rather than a 400.
func TestTranslate_MalformedUUIDIsInvalidNotInternal(t *testing.T) {
	err := translate("player", &pq.Error{Code: "22P02", Message: `invalid input syntax for type uuid: "not-a-uuid"`})

	assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
	assert.Equal(t, "malformed identifier", apperr.Message(err))
}

// TestTranslate_CheckViolationIsInvalid covers the other half of N22: the
// schema's CHECK constraints restate service-layer rules, and tripping one is
// a bad request, not a server fault.
func TestTranslate_CheckViolationIsInvalid(t *testing.T) {
	err := translate("coach spell", &pq.Error{Code: "23514", Constraint: "coach_spells_dates_ordered"})

	assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
}

func TestTranslate_KnownCodesKeepTheirKinds(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want apperr.Kind
	}{
		{"no rows is not found", sql.ErrNoRows, apperr.KindNotFound},
		{"unique violation is conflict", &pq.Error{Code: "23505"}, apperr.KindConflict},
		{"foreign key violation is invalid", &pq.Error{Code: "23503"}, apperr.KindInvalid},
		{"not null violation is invalid", &pq.Error{Code: "23502"}, apperr.KindInvalid},
		// Anything unrecognised must stay internal: a database outage is not
		// the caller's fault and must not be reported as a bad request.
		{"unknown code stays internal", &pq.Error{Code: "08006"}, apperr.KindInternal},
		{"non-driver error stays internal", errors.New("boom"), apperr.KindInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, apperr.KindOf(translate("entity", tt.err)))
		})
	}
}

// TestTranslate_NeverLeaksDriverDetail guards the S3 property against the two
// cases added here: the client-facing message must not carry the SQL fragment
// the driver put in the error.
func TestTranslate_NeverLeaksDriverDetail(t *testing.T) {
	err := translate("player", &pq.Error{
		Code:    "22P02",
		Message: `invalid input syntax for type uuid: "'; DROP TABLE players--"`,
	})

	assert.NotContains(t, apperr.Message(err), "DROP TABLE")
	assert.NotContains(t, apperr.Message(err), "invalid input syntax")
}

func TestSameTeam(t *testing.T) {
	a, b := "team-1", "team-2"

	assert.True(t, sameTeam(nil, nil), "two free agents match")
	assert.True(t, sameTeam(&a, &a))
	assert.False(t, sameTeam(&a, &b))
	assert.False(t, sameTeam(&a, nil), "at a club is not the same as a free agent")
	assert.False(t, sameTeam(nil, &a))
}
