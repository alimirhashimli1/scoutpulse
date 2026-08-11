package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestLogin_UnknownAccountStillHashes pins N5.
//
// Returning early when no account matched skipped the bcrypt comparison
// entirely, so "no such account" answered in microseconds while "wrong
// password" took the ~60-100ms a bcrypt hash costs. Identical response bodies
// do not help when the clock distinguishes them, and account enumeration is
// exactly what the identical bodies were there to prevent.
//
// The assertion is a floor, not a comparison: the repository here is a mock
// that returns instantly, so anything above a few milliseconds can only be the
// hash. That makes the test insensitive to how fast the machine is.
func TestLogin_UnknownAccountStillHashes(t *testing.T) {
	userRepo := new(MockUserRepository)
	userRepo.On("GetByIdentifier", mock.Anything, "nobody@example.com").
		Return(nil, apperr.NotFound("user not found"))

	h := &Handler{UserRepo: userRepo, RefreshRepo: &fakeRefreshRepo{}}

	body, err := json.Marshal(LoginRequest{Identifier: "nobody@example.com", Password: "hunter2hunter2"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	start := time.Now()
	h.Login(rr, req)
	elapsed := time.Since(start)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Greater(t, elapsed, 5*time.Millisecond,
		"the unknown-account path must still pay for a bcrypt comparison, "+
			"or the endpoint leaks which accounts exist by timing")
}

// TestLogin_SameResponseForUnknownAndWrongPassword keeps the other half of the
// property honest: the bodies must not diverge either.
func TestLogin_SameResponseForUnknownAndWrongPassword(t *testing.T) {
	hashed, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
	require.NoError(t, err)

	call := func(identifier, password string, setup func(*MockUserRepository)) (int, string) {
		userRepo := new(MockUserRepository)
		setup(userRepo)
		h := &Handler{UserRepo: userRepo, RefreshRepo: &fakeRefreshRepo{}}

		body, err := json.Marshal(LoginRequest{Identifier: identifier, Password: password})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.Login(rr, req)
		return rr.Code, rr.Body.String()
	}

	unknownCode, unknownBody := call("nobody@example.com", "whatever12345", func(m *MockUserRepository) {
		m.On("GetByIdentifier", mock.Anything, "nobody@example.com").
			Return(nil, apperr.NotFound("user not found"))
	})

	wrongCode, wrongBody := call("real@example.com", "wrong-password", func(m *MockUserRepository) {
		m.On("GetByIdentifier", mock.Anything, "real@example.com").
			Return(&model.User{
				ID:           "test-id",
				Username:     "testuser",
				Email:        "real@example.com",
				PasswordHash: string(hashed),
				Role:         model.UserRole,
			}, nil)
	})

	assert.Equal(t, unknownCode, wrongCode)
	assert.Equal(t, unknownBody, wrongBody,
		"an unknown account and a wrong password must be indistinguishable")
}

// TestDummyPasswordHashMatchesRealCost: the dummy hash only equalises the two
// paths if it costs the same as a stored one. A hardcoded hash at a stale cost
// would reintroduce the timing gap silently.
func TestDummyPasswordHashMatchesRealCost(t *testing.T) {
	cost, err := bcrypt.Cost(dummyPasswordHash)
	require.NoError(t, err, "the dummy value must be a valid bcrypt hash")
	assert.Equal(t, bcrypt.DefaultCost, cost)
}
