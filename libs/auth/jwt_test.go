package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testPrivatePEM, testPublicPEM []byte

func TestMain(m *testing.M) {
	var err error
	testPrivatePEM, testPublicPEM, err = GenerateKeyPair(MinRSAKeyBits)
	if err != nil {
		panic(err)
	}
	if err := SetSigningKey(testPrivatePEM); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// restoreSigner puts the package back into its default test state after a test
// has cleared or replaced the installed keys.
func restoreSigner(t *testing.T) {
	t.Helper()
	resetKeysForTest()
	require.NoError(t, SetSigningKey(testPrivatePEM))
}

func TestGenerateAndValidateToken(t *testing.T) {
	token, err := GenerateToken("user-123", "scout", "admin")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "admin", claims.Role)
	assert.Equal(t, "user-123", claims.Subject)
	assert.Equal(t, Issuer, claims.Issuer)
	assert.NotEmpty(t, claims.ID, "every token needs a jti")
	assert.NotNil(t, claims.ExpiresAt)
}

// TestVerifierCannotMint is the property that asymmetric signing buys: a
// service holding only the public key can check tokens but not create them.
// Under the previous shared HMAC secret, every verifier was also an issuer.
func TestVerifierCannotMint(t *testing.T) {
	issued, err := GenerateToken("user-1", "scout", "admin")
	require.NoError(t, err)

	// Simulate a downstream service: public key only.
	resetKeysForTest()
	t.Cleanup(func() { restoreSigner(t) })
	require.NoError(t, AddVerificationKey(testPublicPEM))

	claims, err := ValidateToken(issued)
	require.NoError(t, err, "a verifier must still be able to check tokens")
	assert.Equal(t, "admin", claims.Role)

	_, err = GenerateToken("attacker", "attacker", "admin")
	assert.ErrorIs(t, err, ErrNoSigningKey, "a verifier must not be able to mint tokens")
}

func TestValidateToken_Rejects(t *testing.T) {
	t.Run("garbage", func(t *testing.T) {
		_, err := ValidateToken("not.a.token")
		assert.Error(t, err)
	})

	t.Run("empty", func(t *testing.T) {
		_, err := ValidateToken("")
		assert.Error(t, err)
	})

	t.Run("signed by a different key", func(t *testing.T) {
		// A token minted with an unrelated key must not verify, even though
		// it is structurally valid.
		otherPriv, _, err := GenerateKeyPair(MinRSAKeyBits)
		require.NoError(t, err)

		resetKeysForTest()
		t.Cleanup(func() { restoreSigner(t) })
		require.NoError(t, SetSigningKey(otherPriv))

		forged, err := GenerateToken("attacker", "attacker", "admin")
		require.NoError(t, err)

		// Now trust only the original key.
		resetKeysForTest()
		require.NoError(t, AddVerificationKey(testPublicPEM))

		_, err = ValidateToken(forged)
		assert.Error(t, err)
	})

	t.Run("no verification key configured", func(t *testing.T) {
		token, err := GenerateToken("user-1", "scout", "user")
		require.NoError(t, err)

		resetKeysForTest()
		t.Cleanup(func() { restoreSigner(t) })

		_, err = ValidateToken(token)
		assert.ErrorIs(t, err, ErrNoVerificationKey, "verification must fail closed")
	})
}

func TestSetSigningKey_Rejects(t *testing.T) {
	t.Run("not PEM", func(t *testing.T) {
		assert.Error(t, SetSigningKey([]byte("plainly not a key")))
	})

	t.Run("undersized key", func(t *testing.T) {
		// GenerateKeyPair floors at MinRSAKeyBits, so build the rejection
		// case by hand.
		assert.Error(t, SetSigningKey([]byte("-----BEGIN PRIVATE KEY-----\nZm9v\n-----END PRIVATE KEY-----\n")))
	})
}

func TestAuthMiddleware(t *testing.T) {
	token, err := GenerateToken("user-456", "scout", "editor")
	require.NoError(t, err)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r.Context())
		assert.True(t, ok)
		assert.Equal(t, "user-456", claims.UserID)
		assert.Equal(t, "editor", claims.Role)
		w.WriteHeader(http.StatusOK)
	})
	middleware := AuthMiddleware(next)

	t.Run("valid token passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("missing header", func(t *testing.T) {
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("malformed header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", token) // missing "Bearer "
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestJWKSRoundTrip(t *testing.T) {
	// The issuer publishes its keys...
	issuer := httptest.NewServer(JWKSHandler())
	defer issuer.Close()

	token, err := GenerateToken("user-789", "scout", "admin")
	require.NoError(t, err)

	resp, err := http.Get(issuer.URL)
	require.NoError(t, err)
	published, err := io.ReadAll(resp.Body)
	require.NoError(t, resp.Body.Close())
	require.NoError(t, err)

	var set JWKS
	require.NoError(t, json.Unmarshal(published, &set))
	require.Len(t, set.Keys, 1)
	assert.Equal(t, "RSA", set.Keys[0].Kty)
	assert.Equal(t, "RS256", set.Keys[0].Alg)
	assert.NotEmpty(t, set.Keys[0].Kid)

	// The published document contains no private material.
	assert.NotContains(t, string(published), "PRIVATE")
	assert.NotContains(t, string(published), `"d"`)

	// Stand in for a separate process: it serves the bytes the issuer
	// published, and holds no keys of its own until it fetches them.
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(published)
	}))
	defer remote.Close()

	resetKeysForTest()
	t.Cleanup(func() { restoreSigner(t) })
	require.NoError(t, LoadJWKS(remote.URL))

	claims, err := ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-789", claims.UserID)

	_, err = GenerateToken("attacker", "attacker", "admin")
	assert.ErrorIs(t, err, ErrNoSigningKey)
}

// TestKeyRotation covers staging a new key: both are trusted, so tokens signed
// with the outgoing key keep working until they expire.
func TestKeyRotation(t *testing.T) {
	oldToken, err := GenerateToken("user-1", "scout", "user")
	require.NoError(t, err)

	newPriv, _, err := GenerateKeyPair(MinRSAKeyBits)
	require.NoError(t, err)

	t.Cleanup(func() { restoreSigner(t) })
	require.NoError(t, SetSigningKey(newPriv)) // starts signing with the new key
	require.NoError(t, AddVerificationKey(testPublicPEM))

	newToken, err := GenerateToken("user-2", "scout", "user")
	require.NoError(t, err)

	for name, token := range map[string]string{"old": oldToken, "new": newToken} {
		claims, err := ValidateToken(token)
		require.NoError(t, err, "%s token should still verify", name)
		assert.NotEmpty(t, claims.UserID)
	}
}

func TestNewRefreshToken(t *testing.T) {
	a, err := NewRefreshToken()
	require.NoError(t, err)
	b, err := NewRefreshToken()
	require.NoError(t, err)

	assert.NotEqual(t, a, b, "refresh tokens must be unpredictable")
	assert.GreaterOrEqual(t, len(a), 40)
}

func TestHasRole(t *testing.T) {
	claims := &Claims{Role: "editor"}
	assert.True(t, claims.HasRole("editor"))
	assert.False(t, claims.HasRole("admin"))
}
