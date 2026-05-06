package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateAndValidateToken(t *testing.T) {
	userID := "user-123"
	role := "ADMIN"

	// 1. Generate token
	token, err := GenerateToken(userID, role)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// 2. Validate token and extract roles
	claims, err := ValidateToken(token)
	assert.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, role, claims.Role)
}

func TestAuthMiddleware(t *testing.T) {
	userID := "user-456"
	role := "USER"
	token, _ := GenerateToken(userID, role)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r.Context())
		assert.True(t, ok)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, role, claims.Role)
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

func TestValidateToken_Invalid(t *testing.T) {
	claims, err := ValidateToken("invalid.token.string")
	assert.Error(t, err)
	assert.Nil(t, claims)

	claims, err = ValidateToken("")
	assert.Error(t, err)
	assert.Nil(t, claims)
}
