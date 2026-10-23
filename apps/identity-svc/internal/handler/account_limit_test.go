package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/libs/platform/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// The per-IP limit on the login route only works while the address it reads
// is honest. The account limit is the one that holds when it is not: guesses
// against one account land in that account's bucket from any address.
func TestLogin_AccountLimitFollowsTheAccountNotTheAddress(t *testing.T) {
	hashed, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	require.NoError(t, err)

	userRepo := new(MockUserRepository)
	userRepo.On("GetByIdentifier", mock.Anything, mock.Anything).Return(&model.User{
		ID: "u1", Username: "ali", Email: "ali@example.com",
		PasswordHash: string(hashed), Role: model.UserRole, EmailVerified: true,
	}, nil)

	h := &Handler{
		UserRepo:     userRepo,
		RefreshRepo:  newFakeRefreshRepo(),
		AccountLimit: server.NewRateLimit(0.0001, 2),
	}

	login := func(identifier, password, ip string) int {
		body, err := json.Marshal(LoginRequest{Identifier: identifier, Password: password})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		req.RemoteAddr = ip + ":5000"
		rr := httptest.NewRecorder()
		h.Login(rr, req)
		return rr.Code
	}

	assert.Equal(t, http.StatusUnauthorized, login("ali@example.com", "guess-one", "198.51.100.1"))
	assert.Equal(t, http.StatusUnauthorized, login("ali@example.com", "guess-two", "198.51.100.2"))
	assert.Equal(t, http.StatusTooManyRequests, login("ali@example.com", "guess-three", "198.51.100.3"),
		"a new address does not buy more guesses")
	assert.Equal(t, http.StatusTooManyRequests, login("  ALI@example.com ", "correct-horse", "198.51.100.4"),
		"case and whitespace do not make a new bucket, and a locked account refuses even the right password")

	assert.Equal(t, http.StatusUnauthorized, login("other@example.com", "guess", "198.51.100.1"),
		"other accounts are unaffected")
}

// Successful sign-ins do not spend the allowance.
func TestLogin_AccountLimitCountsOnlyFailures(t *testing.T) {
	hashed, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	require.NoError(t, err)

	userRepo := new(MockUserRepository)
	userRepo.On("GetByIdentifier", mock.Anything, mock.Anything).Return(&model.User{
		ID: "u1", Username: "ali", Email: "ali@example.com",
		PasswordHash: string(hashed), Role: model.UserRole, EmailVerified: true,
	}, nil)

	h := &Handler{
		UserRepo:     userRepo,
		RefreshRepo:  newFakeRefreshRepo(),
		AccountLimit: server.NewRateLimit(0.0001, 1),
	}

	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(LoginRequest{Identifier: "ali@example.com", Password: "correct-horse"})
		rr := httptest.NewRecorder()
		h.Login(rr, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body)))
		assert.Equal(t, http.StatusOK, rr.Code, "attempt %d", i+1)
	}
}
