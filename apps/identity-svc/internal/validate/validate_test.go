package validate

import (
	"strings"
	"testing"

	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmail(t *testing.T) {
	t.Run("accepts an ordinary address", func(t *testing.T) {
		got, err := Email("someone@example.com")
		require.NoError(t, err)
		assert.Equal(t, "someone@example.com", got)
	})

	t.Run("normalises case", func(t *testing.T) {
		// Addresses are matched against existing accounts when an external
		// sign-in is linked. Without this, "A@example.com" finds no match for
		// "a@example.com" and silently creates a second account for one person.
		got, err := Email("  Someone@Example.COM  ")
		require.NoError(t, err)
		assert.Equal(t, "someone@example.com", got)
	})

	t.Run("rejects what the old check accepted", func(t *testing.T) {
		// Every one of these passed the previous `email == ""` test.
		for _, bad := range []string{
			"not-an-address",
			"@example.com",
			"someone@",
			"someone@localhost", // no dot in the domain
			"someone example.com",
			"<script>alert(1)</script>",
			"'; DROP TABLE users; --",
		} {
			_, err := Email(bad)
			assert.Error(t, err, "should have rejected %q", bad)
			assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
		}
	})

	t.Run("rejects the display-name form", func(t *testing.T) {
		// net/mail parses `Ali <a@example.com>` happily. Storing that whole
		// string as an address would be wrong, and it carries characters that
		// have meaning in an email header.
		_, err := Email("Ali <ali@example.com>")
		assert.Error(t, err)
	})

	t.Run("rejects a header injection attempt", func(t *testing.T) {
		// A newline in an address ends the header and starts another, which is
		// how a Bcc gets added to a message the application thinks it controls.
		_, err := Email("someone@example.com\nBcc: victim@example.com")
		assert.Error(t, err)
	})

	t.Run("rejects an over-long address", func(t *testing.T) {
		_, err := Email(strings.Repeat("a", 250) + "@example.com")
		assert.Error(t, err)
	})
}

func TestUsername(t *testing.T) {
	t.Run("accepts ordinary names", func(t *testing.T) {
		for _, good := range []string{"scout1", "ali.mir", "a_b-c", "Beşiktaş1"} {
			got, err := Username(good)
			require.NoError(t, err, "should have accepted %q", good)
			assert.Equal(t, good, got)
		}
	})

	t.Run("rejects names that would confuse a display", func(t *testing.T) {
		for _, bad := range []string{
			"ab",                    // too short
			strings.Repeat("a", 33), // too long
			"has space",
			"ad	min",
			"<b>bold</b>",
			"a/b",
			".leading",
			"trailing_",
			"",
			"   ",
		} {
			_, err := Username(bad)
			assert.Error(t, err, "should have rejected %q", bad)
		}
	})

	t.Run("counts length in runes, not bytes", func(t *testing.T) {
		// "Ünïtê" is five characters and more than five bytes. Measuring bytes
		// would reject a name its writer counts as well within the limit.
		_, err := Username("Ünïtê")
		assert.NoError(t, err)
	})
}

func TestPassword(t *testing.T) {
	t.Run("enforces the minimum", func(t *testing.T) {
		assert.Error(t, Password("short"))
		assert.NoError(t, Password("longenough"))
	})

	t.Run("rejects a passphrase bcrypt would truncate", func(t *testing.T) {
		// bcrypt hashes at most 72 bytes. Beyond that Go's implementation
		// returns an error, which unchecked becomes a 500 on a registration
		// that looked perfectly reasonable to the person typing it.
		err := Password(strings.Repeat("a", MaxPasswordBytes+1))
		require.Error(t, err)
		assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
	})

	t.Run("counts the limit in bytes", func(t *testing.T) {
		// 40 emoji are 160 bytes: well past bcrypt's limit despite looking
		// like a short passphrase.
		assert.Error(t, Password(strings.Repeat("😀", 40)))
	})

	t.Run("accepts exactly the maximum", func(t *testing.T) {
		assert.NoError(t, Password(strings.Repeat("a", MaxPasswordBytes)))
	})
}
