package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The failure this guards against is silent: the cookies are written happily,
// the browser simply never sends them back, and the callback reports the flow
// as expired without ever having contacted the provider.
func TestCookiePathUsesConfiguredPrefix(t *testing.T) {
	req := func() *http.Request {
		return httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback", nil)
	}

	t.Run("configured prefix is used when no proxy header is sent", func(t *testing.T) {
		// Vercel's rewrites publish the service under /api/identity and send no
		// X-Forwarded-Prefix. This is the case that was broken.
		h := &Handler{OAuth: OAuthDeps{PublicPathPrefix: "/api/identity"}}
		assert.Equal(t, "/api/identity/api/v1/auth", h.cookiePath(req()))
	})

	t.Run("configured prefix wins over the header", func(t *testing.T) {
		h := &Handler{OAuth: OAuthDeps{PublicPathPrefix: "/api/identity"}}
		r := req()
		r.Header.Set("X-Forwarded-Prefix", "/somewhere-else")
		assert.Equal(t, "/api/identity/api/v1/auth", h.cookiePath(r))
	})

	t.Run("falls back to the header when nothing is configured", func(t *testing.T) {
		h := &Handler{}
		r := req()
		r.Header.Set("X-Forwarded-Prefix", "/api/identity")
		assert.Equal(t, "/api/identity/api/v1/auth", h.cookiePath(r))
	})

	t.Run("served at a domain root keeps the bare path", func(t *testing.T) {
		h := &Handler{}
		assert.Equal(t, cookiePath, h.cookiePath(req()))
	})
}
