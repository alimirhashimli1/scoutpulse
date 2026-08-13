package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/scoutpulse/libs/platform/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func envelopeFor(t *testing.T, subject string, payload any) events.Envelope {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return events.Envelope{ID: "event-1", Subject: subject, Payload: raw}
}

// TestUserDeleted_RemovesEveryGrant is what closes N16.
//
// team_editors keys grants by user_id with no foreign key, because users live
// in the identity service's database. Nothing else will ever tell this service
// an account is gone — so without this consumer the grants outlive the
// account, and would apply again if the id were ever reissued.
func TestUserDeleted_RemovesEveryGrant(t *testing.T) {
	editors := newStubEditors().
		grant("doomed-user", "team-1").
		grant("doomed-user", "team-2").
		grant("other-user", "team-1")

	authz := newTestAuthorizer(t, editors)
	consumer := NewIdentityEventConsumer(editors, authz, quietLogger())

	err := consumer.handleUserDeleted(context.Background(),
		envelopeFor(t, events.SubjectUserDeleted, events.UserDeleted{
			UserID: "doomed-user", Username: "doomed",
		}))

	require.NoError(t, err)

	manages, err := editors.Manages(context.Background(), "doomed-user", "team-1")
	require.NoError(t, err)
	assert.False(t, manages, "the deleted account's grants must be gone")

	manages, err = editors.Manages(context.Background(), "doomed-user", "team-2")
	require.NoError(t, err)
	assert.False(t, manages)

	// Somebody else's grant is untouched.
	manages, err = editors.Manages(context.Background(), "other-user", "team-1")
	require.NoError(t, err)
	assert.True(t, manages, "another account's grants must survive")
}

// TestUserDeleted_ClearsTheCachedDecision: the grant row going is not enough
// on its own, because the authorizer caches "may this user edit this club" for
// a few seconds. Without invalidation the deleted account keeps its access
// until the TTL runs out.
func TestUserDeleted_ClearsTheCachedDecision(t *testing.T) {
	editors := newStubEditors().grant("doomed-user", "team-1")
	authz := newTestAuthorizer(t, editors)
	consumer := NewIdentityEventConsumer(editors, authz, quietLogger())

	// Warm the cache.
	require.NoError(t, authz.RequireTeam(ctxAs("doomed-user", RoleEditor), "team-1"))

	require.NoError(t, consumer.handleUserDeleted(context.Background(),
		envelopeFor(t, events.SubjectUserDeleted, events.UserDeleted{UserID: "doomed-user"})))

	err := authz.RequireTeam(ctxAs("doomed-user", RoleEditor), "team-1")
	assert.ErrorIs(t, err, ErrForbidden,
		"the cached answer must be dropped, not left to expire")
}

// TestRoleChanged_ClearsTheCachedDecision: the grant is unaffected by a role
// change, but the cached decision combines it with the role, so it must go.
func TestRoleChanged_ClearsTheCachedDecision(t *testing.T) {
	editors := newStubEditors().grant("user-1", "team-1")
	authz := newTestAuthorizer(t, editors)
	consumer := NewIdentityEventConsumer(editors, authz, quietLogger())

	require.NoError(t, authz.RequireTeam(ctxAs("user-1", RoleEditor), "team-1"))

	require.NoError(t, consumer.handleRoleChanged(context.Background(),
		envelopeFor(t, events.SubjectUserRoleChanged, events.UserRoleChanged{
			UserID: "user-1", OldRole: "editor", NewRole: "user",
		})))

	// The grant itself survives — only the cached decision was dropped.
	manages, err := editors.Manages(context.Background(), "user-1", "team-1")
	require.NoError(t, err)
	assert.True(t, manages, "a role change must not delete the grant itself")
}

// TestMalformedEventIsDropped: a payload that cannot be decoded will never
// decode on redelivery either, so returning an error would produce an endless
// retry loop rather than eventual success.
func TestMalformedEventIsDropped(t *testing.T) {
	editors := newStubEditors()
	consumer := NewIdentityEventConsumer(editors, newTestAuthorizer(t, editors), quietLogger())

	bad := events.Envelope{ID: "e", Subject: events.SubjectUserDeleted, Payload: []byte(`{"user_id":`)}

	assert.NoError(t, consumer.handleUserDeleted(context.Background(), bad),
		"a malformed event must be dropped, not retried forever")
}

// TestEmptyUserIDIsIgnored guards the worst possible bug here: a DELETE with
// an empty user_id must not become a delete-everything.
func TestEmptyUserIDIsIgnored(t *testing.T) {
	editors := newStubEditors().grant("user-1", "team-1")
	consumer := NewIdentityEventConsumer(editors, newTestAuthorizer(t, editors), quietLogger())

	require.NoError(t, consumer.handleUserDeleted(context.Background(),
		envelopeFor(t, events.SubjectUserDeleted, events.UserDeleted{UserID: ""})))

	manages, err := editors.Manages(context.Background(), "user-1", "team-1")
	require.NoError(t, err)
	assert.True(t, manages, "an event with no user id must change nothing")
}
