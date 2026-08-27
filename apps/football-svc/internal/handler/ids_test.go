package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIDsFrom covers the batch-lookup parameter behind ?ids=, which exists so
// the transfer feed can resolve every player on a page in one request instead
// of one per row.
func TestIDsFrom(t *testing.T) {
	parse := func(query string) ([]string, error) {
		return idsFrom(httptest.NewRequest("GET", "/api/v1/players?"+query, nil), "ids")
	}

	t.Run("absent means no filter, not an empty filter", func(t *testing.T) {
		// nil and empty are different downstream: an empty IDs slice must not
		// become `id = ANY('{}')`, which matches nothing and would turn an
		// unfiltered listing into a blank page.
		ids, err := parse("")
		require.NoError(t, err)
		assert.Nil(t, ids)
	})

	t.Run("splits a comma-separated list", func(t *testing.T) {
		ids, err := parse("ids=a,b,c")
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, ids)
	})

	t.Run("drops blanks from stray separators", func(t *testing.T) {
		// A trailing comma is easy to produce when joining ids in a client.
		// Passing the empty string through reaches Postgres as a malformed
		// uuid and returns a 400 that blames the caller for a stray character.
		ids, err := parse("ids=a,,b,")
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, ids)
	})

	t.Run("trims surrounding space", func(t *testing.T) {
		ids, err := parse("ids=a%20,%20b")
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, ids)
	})

	t.Run("refuses more ids than a page could return", func(t *testing.T) {
		// Unbounded, both the query text and the argument array grow with a
		// caller-controlled input.
		_, err := parse("ids=" + strings.TrimSuffix(strings.Repeat("x,", maxIDsPerRequest+1), ","))
		require.Error(t, err)
		assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
	})

	t.Run("accepts exactly the maximum", func(t *testing.T) {
		ids, err := parse("ids=" + strings.TrimSuffix(strings.Repeat("x,", maxIDsPerRequest), ","))
		require.NoError(t, err)
		assert.Len(t, ids, maxIDsPerRequest)
	})
}
