package db

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDSN_EscapesAwkwardPasswords pins the reason the DSN is built through
// net/url rather than fmt.Sprintf.
//
// Concatenating into "password=%s ..." meant a password containing a space
// truncated the DSN, and one containing the right characters could append a
// connection parameter -- overriding sslmode, for instance.
func TestDSN_EscapesAwkwardPasswords(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{"space", "two words"},
		{"single quote", "it's-a-secret"},
		{"ampersand and equals", "a&b=c"},
		{"sslmode injection attempt", "x sslmode=disable"},
		{"url delimiters", "p@ss:w/rd?#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Host: "localhost", Port: "5432",
				User: "svc", Password: tt.password,
				DBName: "app", SSLMode: "require",
			}

			parsed, err := url.Parse(cfg.DSN())
			require.NoError(t, err, "the DSN must remain a parseable URL")

			gotPassword, set := parsed.User.Password()
			require.True(t, set)
			assert.Equal(t, tt.password, gotPassword, "the password must survive the round trip intact")
			assert.Equal(t, "svc", parsed.User.Username())
			assert.Equal(t, "/app", parsed.Path)

			// The caller's sslmode must win regardless of what the password
			// contains.
			assert.Equal(t, "require", parsed.Query().Get("sslmode"))
			assert.Len(t, parsed.Query()["sslmode"], 1, "exactly one sslmode parameter")
		})
	}
}

// TestWithDefaults_ZeroConfigIsStillPooled is the flaw the stack test exposed:
// database/sql treats MaxOpenConns=0 as unlimited, so a Config built by hand
// would have got the unbounded pool these limits exist to prevent.
func TestWithDefaults_ZeroConfigIsStillPooled(t *testing.T) {
	cfg := Config{Host: "localhost", Port: "5432", User: "u", Password: "p", DBName: "d"}.withDefaults()

	assert.Equal(t, DefaultMaxOpenConns, cfg.MaxOpenConns)
	assert.Equal(t, DefaultMaxIdleConns, cfg.MaxIdleConns)
	assert.Equal(t, DefaultConnMaxLifetime, cfg.ConnMaxLifetime)
	assert.Equal(t, DefaultConnMaxIdleTime, cfg.ConnMaxIdleTime)
	assert.Equal(t, "disable", cfg.SSLMode, "an unset sslmode must not produce an empty parameter")
}

func TestWithDefaults_ExplicitValuesAreKept(t *testing.T) {
	cfg := Config{
		SSLMode:         "verify-full",
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: 30 * time.Second,
	}.withDefaults()

	assert.Equal(t, "verify-full", cfg.SSLMode)
	assert.Equal(t, 5, cfg.MaxOpenConns)
	assert.Equal(t, 2, cfg.MaxIdleConns)
	assert.Equal(t, time.Minute, cfg.ConnMaxLifetime)
	assert.Equal(t, 30*time.Second, cfg.ConnMaxIdleTime)
}
