package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// publicPEMOf encodes a public key the way configuration supplies it.
func publicPEMOf(t *testing.T, pub *rsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

// TestAddVerificationKey_RejectsWeakKey pins N6.
//
// The size floor used to apply only to the signing key. A verification key is
// the more dangerous of the two to get wrong: one small enough to factor lets
// an attacker mint tokens this service will accept, and it is the key that
// arrives from outside via configuration or a JWKS document.
func TestAddVerificationKey_RejectsWeakKey(t *testing.T) {
	t.Cleanup(func() { restoreSigner(t) })
	resetKeysForTest()

	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	err = AddVerificationKey(publicPEMOf(t, &weak.PublicKey))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2048 bits")
	assert.Empty(t, verificationKeys, "a rejected key must not be installed")
}

func TestAddVerificationKey_AcceptsStrongKey(t *testing.T) {
	t.Cleanup(func() { restoreSigner(t) })
	resetKeysForTest()

	require.NoError(t, AddVerificationKey(testPublicPEM))
	assert.Len(t, verificationKeys, 1)
}

// TestJWKS_RejectsWeakKey covers the same floor on the path where keys arrive
// from a published document rather than from configuration.
func TestJWKS_RejectsWeakKey(t *testing.T) {
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	jwk := JWK{
		Kty: "RSA", Use: "sig", Alg: "RS256", Kid: "weak",
		N: base64.RawURLEncoding.EncodeToString(weak.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(weak.E)).Bytes()),
	}

	_, err = jwk.toRSAPublicKey()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2048 bits")
}

// TestLoadJWKS_RequiresHTTPSForExternalHosts pins the transport requirement:
// over plain http the key set can be substituted in transit, and these keys
// decide which tokens the service trusts.
func TestLoadJWKS_RequiresHTTPSForExternalHosts(t *testing.T) {
	err := LoadJWKS("http://keys.example.com/.well-known/jwks.json")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}

// staticJWKSServer serves a fixed key set.
//
// JWKSHandler reads the live key map at request time, so it cannot be used to
// test loading: clearing the store to prove the fetch installed something also
// empties what the handler would serve. The document is captured up front and
// served verbatim, and the returned setter lets a test change it to simulate a
// rotation at the issuer.
func staticJWKSServer(t *testing.T, initial JWKS) (*httptest.Server, func(JWKS)) {
	t.Helper()

	var (
		mu     sync.Mutex
		served = initial
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		doc := served
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(srv.Close)

	return srv, func(next JWKS) {
		mu.Lock()
		served = next
		mu.Unlock()
	}
}

// jwksContaining snapshots the key set produced by installing pemKey alone.
func jwksContaining(t *testing.T, publicPEM []byte) JWKS {
	t.Helper()

	saved := snapshotKeys()
	defer restoreKeys(saved)

	resetKeysForTest()
	require.NoError(t, AddVerificationKey(publicPEM))
	return PublicJWKS()
}

func snapshotKeys() map[string]*rsa.PublicKey {
	keysMu.RLock()
	defer keysMu.RUnlock()
	out := make(map[string]*rsa.PublicKey, len(verificationKeys))
	for k, v := range verificationKeys {
		out[k] = v
	}
	return out
}

func restoreKeys(keys map[string]*rsa.PublicKey) {
	keysMu.Lock()
	defer keysMu.Unlock()
	verificationKeys = keys
}

// TestLoadJWKS_AllowsPlainHTTPInCluster is the deliberate exception: loopback
// and in-cluster service names are not routable off the network, and requiring
// certificates for service-to-service traffic would mean nobody enables it.
func TestLoadJWKS_AllowsPlainHTTPInCluster(t *testing.T) {
	t.Cleanup(func() { restoreSigner(t) })

	doc := jwksContaining(t, testPublicPEM)
	srv, _ := staticJWKSServer(t, doc)

	resetKeysForTest()
	require.NoError(t, LoadJWKS(srv.URL))
	assert.Len(t, verificationKeys, 1, "the fetched key must be installed")
}

// TestValidateToken_RefetchesOnUnknownKeyID pins N19.
//
// The comments promised that a rotation propagates and that consumers refetch
// on an unrecognised key id. Neither was implemented: the fetch happened once
// at startup and an unknown kid was a flat rejection, so a rotation required
// restarting every consumer.
func TestValidateToken_RefetchesOnUnknownKeyID(t *testing.T) {
	t.Cleanup(func() { restoreSigner(t) })

	// The issuer rotates to a key this verifier has never seen, and mints a
	// token with it.
	rotatedPrivate, rotatedPublic, err := GenerateKeyPair(MinRSAKeyBits)
	require.NoError(t, err)

	resetKeysForTest()
	require.NoError(t, SetSigningKey(rotatedPrivate))
	token, err := GenerateToken("user-1", "scout", "editor")
	require.NoError(t, err)

	oldDoc := jwksContaining(t, testPublicPEM)
	rotatedDoc := jwksContaining(t, rotatedPublic)

	// The verifier starts holding only the old key, with the JWKS URL
	// configured -- the state every consumer is in the moment a rotation
	// happens at the issuer.
	srv, setServed := staticJWKSServer(t, oldDoc)
	resetKeysForTest()
	require.NoError(t, LoadJWKS(srv.URL))
	require.Len(t, verificationKeys, 1)

	// The issuer publishes the new key. Nothing tells the verifier.
	setServed(rotatedDoc)

	// Open the rate-limit window; a real deployment simply waits it out.
	jwksState.mu.Lock()
	jwksState.lastRefreshed = jwksState.lastRefreshed.Add(-2 * MinJWKSRefreshInterval)
	jwksState.mu.Unlock()

	claims, err := ValidateToken(token)

	require.NoError(t, err, "an unknown kid must trigger a refetch, not a flat rejection")
	assert.Equal(t, "user-1", claims.UserID)
}

// TestRefreshJWKS_IsRateLimited: an unrecognised kid is attacker-controllable,
// so it must not be able to drive one outbound request per token presented.
func TestRefreshJWKS_IsRateLimited(t *testing.T) {
	t.Cleanup(func() { restoreSigner(t) })

	doc := jwksContaining(t, testPublicPEM)

	var fetches int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&fetches, 1)
		_ = json.NewEncoder(w).Encode(doc)
	}))
	defer srv.Close()

	resetKeysForTest()
	require.NoError(t, LoadJWKS(srv.URL))
	require.EqualValues(t, 1, atomic.LoadInt32(&fetches))

	// Every one of these lands inside the window opened by the load above.
	for i := 0; i < 20; i++ {
		require.NoError(t, RefreshJWKS())
	}

	assert.EqualValues(t, 1, atomic.LoadInt32(&fetches),
		"refreshes inside the rate-limit window must not reach the network")
}

// TestAuthMiddleware_ErrorsAreJSON pins N18. Every other error in the platform
// is JSON; a 401 in plain text meant a client needed a second error parser for
// the status it hits most often.
func TestAuthMiddleware_ErrorsAreJSON(t *testing.T) {
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"wrong scheme", "Basic abc123"},
		{"malformed token", "Bearer not-a-jwt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/players", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			require.Equal(t, http.StatusUnauthorized, rr.Code)
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

			var body map[string]string
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body),
				"a 401 body must parse as JSON, like every other error")
			assert.Equal(t, "unauthorized", body["code"])
			assert.NotEmpty(t, body["error"])
		})
	}
}

// TestAuthMiddleware_SchemeIsCaseInsensitive: RFC 7235 defines the auth scheme
// as case-insensitive, and some clients send "bearer".
func TestAuthMiddleware_SchemeIsCaseInsensitive(t *testing.T) {
	token, err := GenerateToken("user-1", "scout", "admin")
	require.NoError(t, err)

	var reached bool
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/players", nil)
	req.Header.Set("Authorization", "bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, reached)
}
