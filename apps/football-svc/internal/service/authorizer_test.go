package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthorizer_CachesGrantLookups checks the hot path does not hit the
// database on every write.
func TestAuthorizer_CachesGrantLookups(t *testing.T) {
	editors := newStubEditors().grant("user-1", "team-1")
	authz := NewAuthorizer(editors)
	ctx := ctxAs("user-1", RoleEditor)

	for i := 0; i < 5; i++ {
		require.NoError(t, authz.RequireTeam(ctx, "team-1"))
	}

	assert.Equal(t, 1, editors.calls, "repeated checks should be served from the cache")
}

// TestAuthorizer_RevocationIsImmediate is the property the whole S4 change
// exists for. When grants lived in the token, a revocation could not take
// effect until it expired -- up to 24 hours later. Now an explicit
// invalidation applies at once.
func TestAuthorizer_RevocationIsImmediate(t *testing.T) {
	editors := newStubEditors().grant("user-1", "team-1")
	authz := NewAuthorizer(editors)
	ctx := ctxAs("user-1", RoleEditor)

	require.NoError(t, authz.RequireTeam(ctx, "team-1"))

	editors.revoke("user-1", "team-1")
	authz.InvalidateUser("user-1")

	assert.ErrorIs(t, authz.RequireTeam(ctx, "team-1"), ErrForbidden)
}

// TestAuthorizer_InvalidateUserIsScoped checks that dropping one user's cached
// grants does not disturb another user's.
func TestAuthorizer_InvalidateUserIsScoped(t *testing.T) {
	editors := newStubEditors().grant("user-1", "team-1").grant("user-12", "team-1")
	authz := NewAuthorizer(editors)

	require.NoError(t, authz.RequireTeam(ctxAs("user-1", RoleEditor), "team-1"))
	require.NoError(t, authz.RequireTeam(ctxAs("user-12", RoleEditor), "team-1"))
	callsAfterWarmup := editors.calls

	// "user-1" is a prefix of "user-12"; a sloppy prefix match would evict
	// both.
	authz.InvalidateUser("user-1")

	require.NoError(t, authz.RequireTeam(ctxAs("user-12", RoleEditor), "team-1"))
	assert.Equal(t, callsAfterWarmup, editors.calls, "user-12's cache entry should survive")
}

func TestAuthorizer_RequireEitherTeam(t *testing.T) {
	editors := newStubEditors().grant("user-1", "team-selling")
	authz := NewAuthorizer(editors)
	ctx := ctxAs("user-1", RoleEditor)

	t.Run("manages the origin", func(t *testing.T) {
		assert.NoError(t, authz.RequireEitherTeam(ctx, ptr("team-selling"), ptr("team-buying")))
	})

	t.Run("manages the destination", func(t *testing.T) {
		editors.grant("user-1", "team-buying")
		authz.InvalidateUser("user-1")
		assert.NoError(t, authz.RequireEitherTeam(ctx, ptr("team-other"), ptr("team-buying")))
	})

	t.Run("manages neither", func(t *testing.T) {
		assert.ErrorIs(t,
			authz.RequireEitherTeam(ctx, ptr("team-x"), ptr("team-y")),
			ErrForbidden)
	})

	t.Run("admin needs no grant", func(t *testing.T) {
		assert.NoError(t, authz.RequireEitherTeam(
			ctxAs("user-admin", RoleAdmin), ptr("team-x"), ptr("team-y")))
	})

	t.Run("plain user is refused", func(t *testing.T) {
		assert.ErrorIs(t,
			authz.RequireEitherTeam(ctxAs("user-plain", "user"), ptr("team-selling"), nil),
			ErrForbidden)
	})
}

func TestAuthorizer_RequireTargetTeam_NilIsAdminOnly(t *testing.T) {
	editors := newStubEditors().grant("user-1", "team-1")
	authz := NewAuthorizer(editors)

	// No club means no club grant can cover it.
	assert.ErrorIs(t, authz.RequireTargetTeam(ctxAs("user-1", RoleEditor), nil), ErrForbidden)
	assert.NoError(t, authz.RequireTargetTeam(ctxAs("user-admin", RoleAdmin), nil))
}
