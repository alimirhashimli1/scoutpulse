package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-signing-key-that-is-long-enough-for-hs256"

func TestMain(m *testing.M) {
	if err := SetSecret([]byte(testSecret)); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestSetSecret_RejectsShortKeys(t *testing.T) {
	err := SetSecret([]byte("too-short"))
	require.Error(t, err)

	// The previously installed key must survive a rejected update.
	token, err := GenerateToken("user-1", "user", nil)
	require.NoError(t, err)
	_, err = ValidateToken(token)
	assert.NoError(t, err)
}

func TestLoadSecretFromEnv(t *testing.T) {
	t.Setenv(SecretEnvVar, "")
	assert.Error(t, LoadSecretFromEnv(), "empty JWT_SECRET must be rejected")

	t.Setenv(SecretEnvVar, testSecret)
	assert.NoError(t, LoadSecretFromEnv())
}

func TestGenerateAndValidateToken(t *testing.T) {
	userID := "user-123"
	role := "admin"
	teams := []string{"team-1", "team-2"}

	// 1. Generate token
	token, err := GenerateToken(userID, role, teams)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// 2. Validate token and extract roles
	claims, err := ValidateToken(token)
	assert.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, role, claims.Role)
	assert.Equal(t, teams, claims.ManagedTeamIDs)
}

func TestAuthMiddleware(t *testing.T) {
	userID := "user-456"
	role := "scout"
	teams := []string{"team-3"}
	token, _ := GenerateToken(userID, role, teams)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r.Context())
		assert.True(t, ok)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, role, claims.Role)
		assert.Equal(t, teams, claims.ManagedTeamIDs)
		w.WriteHeader(http.StatusOK)
	})

	middleware := AuthMiddleware(nextHandler)

	// 1. Success case
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// 2. Missing header
	req = httptest.NewRequest("GET", "/", nil)
	rr = httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	// 3. Invalid token
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rr = httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestPermissions(t *testing.T) {
	adminClaims := &Claims{Role: "admin"}
	scoutClaims := &Claims{Role: "scout", ManagedTeamIDs: []string{"team-A"}}

	assert.True(t, adminClaims.HasRole("admin"))
	assert.False(t, scoutClaims.HasRole("admin"))
	assert.True(t, scoutClaims.HasRole("scout"))

	assert.True(t, adminClaims.HasTeamPermission("any-team"))
	assert.True(t, scoutClaims.HasTeamPermission("team-A"))
	assert.False(t, scoutClaims.HasTeamPermission("team-B"))
}

func TestValidateToken_Invalid(t *testing.T) {
	claims, err := ValidateToken("invalid.token.string")
	assert.Error(t, err)
	assert.Nil(t, claims)

	claims, err = ValidateToken("")
	assert.Error(t, err)
	assert.Nil(t, claims)
}
