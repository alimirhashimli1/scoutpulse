package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/identity-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestMain installs a signing key. This service is the token issuer, so it
// holds the private half.
func TestMain(m *testing.M) {
	privatePEM, _, err := auth.GenerateKeyPair(auth.MinRSAKeyBits)
	if err != nil {
		panic(err)
	}
	if err := auth.SetSigningKey(privatePEM); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// --- mocks -------------------------------------------------------------

type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) Create(ctx context.Context, user *model.User) error {
	return m.Called(ctx, user).Error(0)
}

func (m *MockUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *MockUserRepository) GetByIdentifier(ctx context.Context, identifier string) (*model.User, error) {
	args := m.Called(ctx, identifier)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *MockUserRepository) UpdateRole(ctx context.Context, userID string, role model.Role, updatedBy string) error {
	return m.Called(ctx, userID, role, updatedBy).Error(0)
}

// fakeRefreshRepo is an in-memory session store.
type fakeRefreshRepo struct {
	mu        sync.Mutex
	byHash    map[string]*model.RefreshToken
	revokeAll int
	nextID    int
}

func newFakeRefreshRepo() *fakeRefreshRepo {
	return &fakeRefreshRepo{byHash: map[string]*model.RefreshToken{}}
}

func (f *fakeRefreshRepo) Create(_ context.Context, t *model.RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	t.ID = string(rune('a' + f.nextID))
	t.IssuedAt = time.Now()
	f.byHash[string(t.TokenHash)] = t
	return nil
}

func (f *fakeRefreshRepo) GetByToken(_ context.Context, token string) (*model.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byHash[string(repository.HashToken(token))]
	if !ok {
		return nil, apperr.Unauthorized("invalid refresh token")
	}
	return t, nil
}

func (f *fakeRefreshRepo) Rotate(_ context.Context, oldID string, next *model.RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, t := range f.byHash {
		if t.ID == oldID {
			if t.RevokedAt != nil {
				return apperr.Unauthorized("invalid refresh token")
			}
			now := time.Now()
			t.RevokedAt = &now
		}
	}

	f.nextID++
	next.ID = string(rune('a' + f.nextID))
	next.IssuedAt = time.Now()
	f.byHash[string(next.TokenHash)] = next
	return nil
}

func (f *fakeRefreshRepo) Revoke(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.byHash[string(repository.HashToken(token))]; ok {
		now := time.Now()
		t.RevokedAt = &now
	}
	return nil
}

func (f *fakeRefreshRepo) RevokeAllForUser(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokeAll++
	for _, t := range f.byHash {
		if t.UserID == userID {
			now := time.Now()
			t.RevokedAt = &now
		}
	}
	return nil
}

func (f *fakeRefreshRepo) DeleteExpired(context.Context, time.Time) (int64, error) { return 0, nil }

// --- helpers ------------------------------------------------------------

func postJSON(t *testing.T, h http.HandlerFunc, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func testUser(t *testing.T, password string, role model.Role) *model.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	return &model.User{
		ID:           "test-id",
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: string(hash),
		Role:         role,
	}
}

// --- tests --------------------------------------------------------------

func TestHealth(t *testing.T) {
	h := &Handler{}
	rr := httptest.NewRecorder()
	h.Health(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "Identity Service is healthy", rr.Body.String())
}

func TestRegister(t *testing.T) {
	repo := new(MockUserRepository)
	h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

	var created *model.User
	repo.On("Create", mock.Anything, mock.AnythingOfType("*model.User")).Return(nil).Once().
		Run(func(args mock.Arguments) { created = args.Get(1).(*model.User) })

	rr := postJSON(t, h.Register, "/api/v1/auth/register", RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	})

	assert.Equal(t, http.StatusCreated, rr.Code)
	require.NotNil(t, created)
	assert.Equal(t, "testuser", created.Username)
	assert.NotEqual(t, "password123", created.PasswordHash, "password must not be stored in plain text")
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(created.PasswordHash), []byte("password123")))

	// The response must not carry the hash.
	assert.NotContains(t, rr.Body.String(), "password_hash")
	repo.AssertExpectations(t)
}

// TestRegister_IgnoresClientSuppliedRole is a regression test for the
// privilege-escalation hole where Register trusted a "role" field from the
// request body, letting anyone create an admin account.
func TestRegister_IgnoresClientSuppliedRole(t *testing.T) {
	repo := new(MockUserRepository)
	h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

	var created *model.User
	repo.On("Create", mock.Anything, mock.AnythingOfType("*model.User")).Return(nil).Maybe().
		Run(func(args mock.Arguments) { created = args.Get(1).(*model.User) })

	body := []byte(`{"username":"attacker","email":"a@example.com","password":"password123","role":"admin"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.Register(rr, req)

	// DecodeJSON rejects unknown fields, so the request is refused outright.
	// Even if that guard were relaxed, the assertion below must hold.
	if rr.Code == http.StatusCreated {
		require.NotNil(t, created)
		assert.Equal(t, model.UserRole, created.Role,
			"self-registration must never assign a privileged role")
	} else {
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	}
}

func TestRegister_RejectsShortPassword(t *testing.T) {
	repo := new(MockUserRepository)
	h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

	rr := postJSON(t, h.Register, "/api/v1/auth/register", RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "short",
	})

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestLogin(t *testing.T) {
	const password = "password123"
	user := testUser(t, password, model.UserRole)

	t.Run("success returns an access and refresh pair", func(t *testing.T) {
		repo := new(MockUserRepository)
		h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}
		repo.On("GetByIdentifier", mock.Anything, user.Email).Return(user, nil)

		rr := postJSON(t, h.Login, "/api/v1/auth/login",
			LoginRequest{Identifier: user.Email, Password: password})

		require.Equal(t, http.StatusOK, rr.Code)

		var resp TokenResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, "Bearer", resp.TokenType)
		assert.Equal(t, int(auth.AccessTokenTTL.Seconds()), resp.ExpiresIn)

		claims, err := auth.ValidateToken(resp.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, user.ID, claims.UserID)
		assert.Equal(t, string(model.UserRole), claims.Role)
	})

	t.Run("wrong password", func(t *testing.T) {
		repo := new(MockUserRepository)
		h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}
		repo.On("GetByIdentifier", mock.Anything, user.Email).Return(user, nil)

		rr := postJSON(t, h.Login, "/api/v1/auth/login",
			LoginRequest{Identifier: user.Email, Password: "wrongpassword"})

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("unknown user is indistinguishable from a wrong password", func(t *testing.T) {
		repo := new(MockUserRepository)
		h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}
		repo.On("GetByIdentifier", mock.Anything, "nobody@example.com").
			Return(nil, apperr.NotFound("user not found"))

		rr := postJSON(t, h.Login, "/api/v1/auth/login",
			LoginRequest{Identifier: "nobody@example.com", Password: password})

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		// Revealing "no such user" would let anyone enumerate accounts.
		assert.NotContains(t, rr.Body.String(), "not found")
	})
}

// TestRefreshRotatesToken checks the refresh token is single-use.
func TestRefreshRotatesToken(t *testing.T) {
	const password = "password123"
	user := testUser(t, password, model.UserRole)

	repo := new(MockUserRepository)
	sessions := newFakeRefreshRepo()
	h := &Handler{UserRepo: repo, RefreshRepo: sessions}
	repo.On("GetByIdentifier", mock.Anything, user.Email).Return(user, nil)
	repo.On("GetByID", mock.Anything, user.ID).Return(user, nil)

	rr := postJSON(t, h.Login, "/api/v1/auth/login",
		LoginRequest{Identifier: user.Email, Password: password})
	var first TokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &first))

	rr = postJSON(t, h.Refresh, "/api/v1/auth/refresh",
		RefreshRequest{RefreshToken: first.RefreshToken})
	require.Equal(t, http.StatusOK, rr.Code)

	var second TokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &second))
	assert.NotEqual(t, first.RefreshToken, second.RefreshToken, "the refresh token must rotate")
	assert.NotEmpty(t, second.AccessToken)
}

// TestRefreshReuseRevokesEverySession covers leak detection: presenting a
// refresh token that has already been exchanged means a copy is circulating,
// and the response is to end every session rather than keep serving both the
// real client and whoever else holds it.
func TestRefreshReuseRevokesEverySession(t *testing.T) {
	const password = "password123"
	user := testUser(t, password, model.UserRole)

	repo := new(MockUserRepository)
	sessions := newFakeRefreshRepo()
	h := &Handler{UserRepo: repo, RefreshRepo: sessions}
	repo.On("GetByIdentifier", mock.Anything, user.Email).Return(user, nil)
	repo.On("GetByID", mock.Anything, user.ID).Return(user, nil)

	rr := postJSON(t, h.Login, "/api/v1/auth/login",
		LoginRequest{Identifier: user.Email, Password: password})
	var first TokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &first))

	// Legitimate refresh.
	rr = postJSON(t, h.Refresh, "/api/v1/auth/refresh",
		RefreshRequest{RefreshToken: first.RefreshToken})
	require.Equal(t, http.StatusOK, rr.Code)
	var second TokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &second))

	// Replay of the now-revoked token.
	rr = postJSON(t, h.Refresh, "/api/v1/auth/refresh",
		RefreshRequest{RefreshToken: first.RefreshToken})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Positive(t, sessions.revokeAll, "reuse should revoke the user's sessions")

	// The token issued to the real client is dead too: the safe assumption
	// is that the attacker has it.
	rr = postJSON(t, h.Refresh, "/api/v1/auth/refresh",
		RefreshRequest{RefreshToken: second.RefreshToken})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestRefreshRejectsUnknownToken(t *testing.T) {
	h := &Handler{UserRepo: new(MockUserRepository), RefreshRepo: newFakeRefreshRepo()}

	rr := postJSON(t, h.Refresh, "/api/v1/auth/refresh",
		RefreshRequest{RefreshToken: "not-a-real-token"})

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestLogoutRevokesToken(t *testing.T) {
	const password = "password123"
	user := testUser(t, password, model.UserRole)

	repo := new(MockUserRepository)
	sessions := newFakeRefreshRepo()
	h := &Handler{UserRepo: repo, RefreshRepo: sessions}
	repo.On("GetByIdentifier", mock.Anything, user.Email).Return(user, nil)
	repo.On("GetByID", mock.Anything, user.ID).Return(user, nil)

	rr := postJSON(t, h.Login, "/api/v1/auth/login",
		LoginRequest{Identifier: user.Email, Password: password})
	var tokens TokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &tokens))

	rr = postJSON(t, h.Logout, "/api/v1/auth/logout",
		RefreshRequest{RefreshToken: tokens.RefreshToken})
	assert.Equal(t, http.StatusNoContent, rr.Code)

	// The revoked token can no longer be exchanged.
	rr = postJSON(t, h.Refresh, "/api/v1/auth/refresh",
		RefreshRequest{RefreshToken: tokens.RefreshToken})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestUpdateRole(t *testing.T) {
	newAdminCtx := func() context.Context {
		return context.WithValue(context.Background(), auth.ClaimsContextKey,
			&auth.Claims{UserID: "admin-1", Role: string(model.AdminRole)})
	}

	t.Run("admin may promote", func(t *testing.T) {
		repo := new(MockUserRepository)
		sessions := newFakeRefreshRepo()
		h := &Handler{UserRepo: repo, RefreshRepo: sessions}
		repo.On("UpdateRole", mock.Anything, "user-2", model.EditorRole, "admin-1").Return(nil).Once()

		body, _ := json.Marshal(UpdateRoleRequest{Role: model.EditorRole})
		req := httptest.NewRequest(http.MethodPut, "/api/v1/users/user-2/role", bytes.NewReader(body)).
			WithContext(newAdminCtx())
		req.SetPathValue("id", "user-2")
		rr := httptest.NewRecorder()

		h.UpdateRole(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		// The old role is baked into any token already issued, so the
		// user's sessions must end.
		assert.Positive(t, sessions.revokeAll, "a role change must end existing sessions")
		repo.AssertExpectations(t)
	})

	t.Run("non-admin is refused", func(t *testing.T) {
		repo := new(MockUserRepository)
		h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey,
			&auth.Claims{UserID: "editor-1", Role: string(model.EditorRole)})

		body, _ := json.Marshal(UpdateRoleRequest{Role: model.AdminRole})
		req := httptest.NewRequest(http.MethodPut, "/api/v1/users/editor-1/role", bytes.NewReader(body)).
			WithContext(ctx)
		req.SetPathValue("id", "editor-1")
		rr := httptest.NewRecorder()

		h.UpdateRole(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		repo.AssertNotCalled(t, "UpdateRole", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("unknown role is rejected", func(t *testing.T) {
		repo := new(MockUserRepository)
		h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

		body := []byte(`{"role":"superuser"}`)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/users/user-2/role", bytes.NewReader(body)).
			WithContext(newAdminCtx())
		req.SetPathValue("id", "user-2")
		rr := httptest.NewRecorder()

		h.UpdateRole(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		repo.AssertNotCalled(t, "UpdateRole", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}
