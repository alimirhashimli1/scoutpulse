package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
