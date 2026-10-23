package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// cookieClient drives the handlers the way the browser does: it sends the
// opt-in header and carries the refresh cookie between calls.
type cookieClient struct {
	t      *testing.T
	cookie *http.Cookie
}

func (c *cookieClient) post(handler http.HandlerFunc, path string, body any) *httptest.ResponseRecorder {
	c.t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(c.t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SessionCookieHeader, "1")
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	rr := httptest.NewRecorder()
	handler(rr, req)
	for _, ck := range rr.Result().Cookies() {
		if ck.Name == refreshCookie {
			if ck.MaxAge < 0 {
				c.cookie = nil
			} else {
				c.cookie = ck
			}
		}
	}
	return rr
}

func cookieHandler(t *testing.T) (*Handler, *fakeRefreshRepo, *model.User) {
	t.Helper()
	const password = "password123"
	user := testUser(t, password, model.UserRole)
	repo := new(MockUserRepository)
	repo.On("GetByIdentifier", mock.Anything, user.Email).Return(user, nil)
	repo.On("GetByID", mock.Anything, user.ID).Return(user, nil)
	sessions := newFakeRefreshRepo()
	h := &Handler{
		UserRepo: repo, RefreshRepo: sessions,
		OAuth: OAuthDeps{SecureCookies: true, PublicPathPrefix: "/api/identity"},
	}
	return h, sessions, user
}

func TestSessionCookie_LoginKeepsTheRefreshTokenOutOfTheBody(t *testing.T) {
	h, _, user := cookieHandler(t)
	c := &cookieClient{t: t}

	rr := c.post(h.Login, "/api/v1/auth/login", LoginRequest{Identifier: user.Email, Password: "password123"})

	require.Equal(t, http.StatusOK, rr.Code)
	assert.NotContains(t, rr.Body.String(), "refresh_token", "a script must have nothing to read")
	assert.Contains(t, rr.Body.String(), "access_token")

	require.NotNil(t, c.cookie)
	assert.True(t, c.cookie.HttpOnly)
	assert.True(t, c.cookie.Secure)
	assert.Equal(t, http.SameSiteStrictMode, c.cookie.SameSite)
	assert.Equal(t, "/api/identity/api/v1/auth", c.cookie.Path, "sent to the auth endpoints only")
}

func TestSessionCookie_RefreshAndLogoutUseTheCookie(t *testing.T) {
	h, _, user := cookieHandler(t)
	c := &cookieClient{t: t}
	c.post(h.Login, "/api/v1/auth/login", LoginRequest{Identifier: user.Email, Password: "password123"})
	first := c.cookie.Value

	rr := c.post(h.Refresh, "/api/v1/auth/refresh", RefreshRequest{})
	require.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, c.cookie)
	assert.NotEqual(t, first, c.cookie.Value, "rotated into a new cookie")
	assert.NotContains(t, rr.Body.String(), "refresh_token")

	afterRefresh := c.cookie
	rr = c.post(h.Logout, "/api/v1/auth/logout", RefreshRequest{})
	require.Equal(t, http.StatusNoContent, rr.Code)
	assert.Nil(t, c.cookie, "logout clears the cookie")

	c.cookie = afterRefresh
	rr = c.post(h.Refresh, "/api/v1/auth/refresh", RefreshRequest{})
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "and revokes the token it held")
}

// The opt-in header is what a cross-site page cannot add, so without it the
// cookie is never read.
func TestSessionCookie_IgnoredWithoutTheHeader(t *testing.T) {
	h, _, user := cookieHandler(t)
	c := &cookieClient{t: t}
	c.post(h.Login, "/api/v1/auth/login", LoginRequest{Identifier: user.Email, Password: "password123"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(c.cookie)
	rr := httptest.NewRecorder()
	h.Refresh(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// A browser still holding a token from before the cookie presents it in the
// body once and is moved onto the cookie.
func TestSessionCookie_LegacyBodyTokenMovesToTheCookie(t *testing.T) {
	h, _, user := cookieHandler(t)
	legacy := postJSON(t, h.Login, "/api/v1/auth/login",
		LoginRequest{Identifier: user.Email, Password: "password123"})
	var pair TokenResponse
	require.NoError(t, json.Unmarshal(legacy.Body.Bytes(), &pair))
	require.NotEmpty(t, pair.RefreshToken, "clients without the header still get it in the body")

	c := &cookieClient{t: t}
	rr := c.post(h.Refresh, "/api/v1/auth/refresh", RefreshRequest{RefreshToken: pair.RefreshToken})

	require.Equal(t, http.StatusOK, rr.Code)
	assert.NotNil(t, c.cookie)
	assert.NotContains(t, rr.Body.String(), "refresh_token")
}

// Two tabs refreshing together present the same cookie. The second is refused,
// but must not end every session the user holds.
func TestRefresh_ConcurrentTabsDoNotRevokeEverything(t *testing.T) {
	h, sessions, user := cookieHandler(t)
	c := &cookieClient{t: t}
	c.post(h.Login, "/api/v1/auth/login", LoginRequest{Identifier: user.Email, Password: "password123"})
	shared := c.cookie

	rr := c.post(h.Refresh, "/api/v1/auth/refresh", RefreshRequest{})
	require.Equal(t, http.StatusOK, rr.Code)
	winner := c.cookie

	loser := &cookieClient{t: t, cookie: shared}
	rr = loser.post(h.Refresh, "/api/v1/auth/refresh", RefreshRequest{})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Zero(t, sessions.revokeAll, "a race is not a theft")

	retry := &cookieClient{t: t, cookie: winner}
	rr = retry.post(h.Refresh, "/api/v1/auth/refresh", RefreshRequest{})
	assert.Equal(t, http.StatusOK, rr.Code, "the losing tab recovers with the cookie the winner set")
}

// With PUBLIC_BASE_URL unset there is no configured prefix. The cookie must
// still land on the path the browser calls, and be Secure over https, or every
// reload would sign the user out.
func TestSessionCookie_PathAndSecureWithoutConfiguration(t *testing.T) {
	h, _, user := cookieHandler(t)
	h.OAuth = OAuthDeps{}

	raw, _ := json.Marshal(LoginRequest{Identifier: user.Email, Password: "password123"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(raw))
	req.Header.Set(SessionCookieHeader, "1")
	req.Header.Set("X-Forwarded-Prefix", "/api/identity")
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var got *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == refreshCookie {
			got = c
		}
	}
	require.NotNil(t, got)
	assert.Equal(t, "/api/identity/api/v1/auth", got.Path)
	assert.True(t, got.Secure)
}

// End to end through a real cookie jar, behind a proxy that mounts the service
// under /api/identity the way the Vercel rewrite does, over TLS. This is the
// browser's view: whether the cookie it was given is the one it sends back.
func TestSessionCookie_RoundTripsThroughAMountedProxy(t *testing.T) {
	h, _, user := cookieHandler(t)
	h.OAuth = OAuthDeps{} // nothing configured: the client's prefix must carry it

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", h.Refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", h.Logout)
	site := httptest.NewTLSServer(http.StripPrefix("/api/identity", mux))
	defer site.Close()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := site.Client()
	client.Jar = jar

	call := func(path string, body any) *http.Response {
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, site.URL+"/api/identity/api/v1/auth/"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(SessionCookieHeader, "1")
		req.Header.Set("X-Forwarded-Prefix", "/api/identity")
		res, err := client.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = res.Body.Close() })
		return res
	}

	require.Equal(t, http.StatusOK, call("login", LoginRequest{Identifier: user.Email, Password: "password123"}).StatusCode)
	authURL, _ := url.Parse(site.URL + "/api/identity/api/v1/auth/refresh")
	require.Len(t, jar.Cookies(authURL), 1, "the jar holds the cookie for the auth path")
	otherURL, _ := url.Parse(site.URL + "/api/football/api/v1/players")
	assert.Empty(t, jar.Cookies(otherURL), "and does not send it anywhere else")

	assert.Equal(t, http.StatusOK, call("refresh", RefreshRequest{}).StatusCode, "first reload")
	assert.Equal(t, http.StatusOK, call("refresh", RefreshRequest{}).StatusCode, "second reload, rotated cookie")

	assert.Equal(t, http.StatusNoContent, call("logout", RefreshRequest{}).StatusCode)
	assert.Empty(t, jar.Cookies(authURL), "logout removes it")
	assert.Equal(t, http.StatusBadRequest, call("refresh", RefreshRequest{}).StatusCode, "nothing left to refresh with")
}
