package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/identity-svc/internal/oauth"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- fakes -------------------------------------------------------------

type fakeIdentityRepo struct {
	mock.Mock
}

func (f *fakeIdentityRepo) GetByProviderAccount(ctx context.Context, provider, providerUserID string) (*model.Identity, error) {
	args := f.Called(ctx, provider, providerUserID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Identity), args.Error(1)
}

func (f *fakeIdentityRepo) ListForUser(ctx context.Context, userID string) ([]model.Identity, error) {
	args := f.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.Identity), args.Error(1)
}

func (f *fakeIdentityRepo) Link(ctx context.Context, i *model.Identity) error {
	return f.Called(ctx, i).Error(0)
}

func (f *fakeIdentityRepo) Unlink(ctx context.Context, userID, provider string) error {
	return f.Called(ctx, userID, provider).Error(0)
}

func (f *fakeIdentityRepo) TouchLogin(ctx context.Context, id string) error {
	return f.Called(ctx, id).Error(0)
}

type fakeLoginCodes struct{ mock.Mock }

func (f *fakeLoginCodes) Issue(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	args := f.Called(ctx, userID, ttl)
	return args.String(0), args.Error(1)
}

func (f *fakeLoginCodes) Redeem(ctx context.Context, code string) (string, error) {
	args := f.Called(ctx, code)
	return args.String(0), args.Error(1)
}

func (f *fakeLoginCodes) DeleteExpired(context.Context, time.Time) (int64, error) { return 0, nil }

func oauthHandler(users *MockUserRepository, identities *fakeIdentityRepo) *Handler {
	return &Handler{
		UserRepo:    users,
		RefreshRepo: newFakeRefreshRepo(),
		OAuth: OAuthDeps{
			Providers:   oauth.FromEnv("http://localhost:8080"),
			Identities:  identities,
			LoginCodes:  &fakeLoginCodes{},
			FrontendURL: "http://localhost:4200",
		},
	}
}

func req(t *testing.T) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback", nil)
}

// --- the linking policy ------------------------------------------------

// TestResolveAccount_LinksOnlyWhenTheProviderVerifiedTheEmail is the security
// decision this whole feature turns on.
//
// If an unverified provider email were accepted as proof of ownership, anyone
// who could register that address at the provider could take over the matching
// local account without ever knowing its password.
func TestResolveAccount_LinksOnlyWhenTheProviderVerifiedTheEmail(t *testing.T) {
	t.Run("verified email links to the existing account", func(t *testing.T) {
		users := new(MockUserRepository)
		identities := new(fakeIdentityRepo)

		identities.On("GetByProviderAccount", mock.Anything, "google", "google-sub-1").
			Return(nil, apperr.NotFound("identity not found"))
		users.On("GetByIdentifier", mock.Anything, "ali@example.test").
			Return(&model.User{ID: "user-1", Email: "ali@example.test"}, nil)
		identities.On("Link", mock.Anything, mock.Anything).Return(nil).Once()

		h := oauthHandler(users, identities)
		user, err := h.resolveAccount(req(t), oauth.Google, &oauth.Profile{
			ProviderUserID: "google-sub-1",
			Email:          "ali@example.test",
			EmailVerified:  true,
		})

		require.NoError(t, err)
		assert.Equal(t, "user-1", user.ID)
		identities.AssertExpectations(t)
	})

	t.Run("unverified email is refused, not linked", func(t *testing.T) {
		users := new(MockUserRepository)
		identities := new(fakeIdentityRepo)

		identities.On("GetByProviderAccount", mock.Anything, "facebook", "fb-1").
			Return(nil, apperr.NotFound("identity not found"))
		users.On("GetByIdentifier", mock.Anything, "ali@example.test").
			Return(&model.User{ID: "user-1", Email: "ali@example.test"}, nil)

		h := oauthHandler(users, identities)
		_, err := h.resolveAccount(req(t), oauth.Facebook, &oauth.Profile{
			ProviderUserID: "fb-1",
			Email:          "ali@example.test",
			EmailVerified:  false,
		})

		require.Error(t, err)
		assert.Equal(t, apperr.KindConflict, apperr.KindOf(err),
			"an unverified provider email must not claim an existing account")
		identities.AssertNotCalled(t, "Link", mock.Anything, mock.Anything)
		users.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})
}

// TestResolveAccount_KeyedOnProviderIDNotEmail: a provider account that is
// already linked signs in on that link alone. Emails at a provider can be
// changed or reassigned; the subject id cannot.
func TestResolveAccount_KeyedOnProviderIDNotEmail(t *testing.T) {
	users := new(MockUserRepository)
	identities := new(fakeIdentityRepo)

	identities.On("GetByProviderAccount", mock.Anything, "google", "google-sub-1").
		Return(&model.Identity{ID: "identity-1", UserID: "user-1"}, nil)
	identities.On("TouchLogin", mock.Anything, "identity-1").Return(nil)
	users.On("GetByID", mock.Anything, "user-1").
		Return(&model.User{ID: "user-1"}, nil)

	h := oauthHandler(users, identities)
	// The email now differs from whatever was recorded at link time.
	user, err := h.resolveAccount(req(t), oauth.Google, &oauth.Profile{
		ProviderUserID: "google-sub-1",
		Email:          "a-completely-different@example.test",
		EmailVerified:  true,
	})

	require.NoError(t, err)
	assert.Equal(t, "user-1", user.ID)
	// No email lookup should have happened at all.
	users.AssertNotCalled(t, "GetByIdentifier", mock.Anything, mock.Anything)
}

// TestResolveAccount_CreatesAPasswordlessUser: a first-time provider sign-in
// creates an ordinary account with no password and the lowest role.
func TestResolveAccount_CreatesAPasswordlessUser(t *testing.T) {
	users := new(MockUserRepository)
	identities := new(fakeIdentityRepo)

	identities.On("GetByProviderAccount", mock.Anything, "google", "google-sub-9").
		Return(nil, apperr.NotFound("identity not found"))
	users.On("GetByIdentifier", mock.Anything, "new@example.test").
		Return(nil, apperr.NotFound("user not found"))
	// The username-availability probe.
	users.On("GetByIdentifier", mock.Anything, mock.Anything).
		Return(nil, apperr.NotFound("user not found"))

	var created *model.User
	users.On("Create", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { created = args.Get(1).(*model.User) }).
		Return(nil).Once()
	identities.On("Link", mock.Anything, mock.Anything).Return(nil).Once()

	h := oauthHandler(users, identities)
	_, err := h.resolveAccount(req(t), oauth.Google, &oauth.Profile{
		ProviderUserID: "google-sub-9",
		Email:          "new@example.test",
		EmailVerified:  true,
		Name:           "New Person",
	})

	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Empty(t, created.PasswordHash, "a provider account must have no password")
	assert.Equal(t, model.UserRole, created.Role,
		"signing in with a provider must grant no more than self-registration")
}

// --- username derivation ----------------------------------------------

func TestSanitiseUsername(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Ali Mirhashimli", "ali-mirhashimli"},
		{"  Spaced  Out  ", "spaced--out"},
		{"UPPER", "upper"},
		{"weird!@#$%chars", "weirdchars"},
		{"-leading-and-trailing-", "leading-and-trailing"},
		{"!!!", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitiseUsername(tt.in))
		})
	}
}

func TestSanitiseUsername_BoundsLength(t *testing.T) {
	long := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	assert.Len(t, sanitiseUsername(long), 32)
}

// --- provider registry -------------------------------------------------

// TestRegistry_OmitsUnconfiguredProviders: a sign-in page should only offer
// buttons that will work, and an unconfigured provider must 404 rather than
// redirect somewhere broken.
func TestRegistry_OmitsUnconfiguredProviders(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("FACEBOOK_CLIENT_ID", "")
	t.Setenv("FACEBOOK_CLIENT_SECRET", "")

	r := oauth.FromEnv("http://localhost:8080")

	assert.Equal(t, []oauth.Name{oauth.Google}, r.Configured())
	_, ok := r.Get(oauth.Facebook)
	assert.False(t, ok, "a provider with no credentials must not be offered")
}

func TestParseName(t *testing.T) {
	for _, valid := range []string{"google", "facebook"} {
		_, ok := oauth.ParseName(valid)
		assert.True(t, ok, valid)
	}
	for _, invalid := range []string{"", "github", "GOOGLE", "../admin"} {
		_, ok := oauth.ParseName(invalid)
		assert.False(t, ok, invalid)
	}
}

// TestCookiePathMatchesTheBrowsersURL covers the bug that made external
// sign-in fail every time it was attempted through the gateway.
//
// The service sees /api/v1/auth/... because the gateway strips its prefix, but
// the browser's URL is /api/identity/api/v1/auth/... A cookie scoped to the
// service's own path is never sent back, so the callback finds no state and
// reports "expired" -- in under a millisecond, without ever reaching the
// provider, and with nothing in the log to say why.
func TestCookiePathMatchesTheBrowsersURL(t *testing.T) {
	t.Run("behind the gateway, the public prefix is included", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/api/v1/auth/google", nil)
		r.Header.Set("X-Forwarded-Prefix", "/api/identity")

		path := cookiePathFor(r)

		assert.Equal(t, "/api/identity/api/v1/auth", path)
		// The property that actually matters: the browser only returns a
		// cookie whose path is a prefix of the URL it is requesting.
		assert.True(t, strings.HasPrefix("/api/identity/api/v1/auth/google/callback", path),
			"the callback URL must sit under the cookie path, or the cookie is never sent")
	})

	t.Run("reached directly, the service path is used unchanged", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/api/v1/auth/google", nil)

		path := cookiePathFor(r)

		assert.Equal(t, "/api/v1/auth", path)
		assert.True(t, strings.HasPrefix("/api/v1/auth/google/callback", path))
	})

	t.Run("a trailing slash does not double up", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/api/v1/auth/google", nil)
		r.Header.Set("X-Forwarded-Prefix", "/api/identity/")

		assert.Equal(t, "/api/identity/api/v1/auth", cookiePathFor(r))
	})

	t.Run("a malformed prefix is ignored rather than built on", func(t *testing.T) {
		// The header is client-controllable when the gateway does not overwrite
		// it. It can only ever mis-scope a short-lived HttpOnly cookie for the
		// client that sent it, but a value carrying a newline has no business
		// reaching a Set-Cookie header.
		for _, bad := range []string{"api/identity", "https://evil.example", "/x\r\nSet-Cookie: a=b"} {
			r := httptest.NewRequest("GET", "/api/v1/auth/google", nil)
			r.Header.Set("X-Forwarded-Prefix", bad)
			assert.Equal(t, cookiePath, cookiePathFor(r), "should have ignored %q", bad)
		}
	})
}
