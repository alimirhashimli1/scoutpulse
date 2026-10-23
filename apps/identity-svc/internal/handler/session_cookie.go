package handler

import (
	"net/http"
	"time"

	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/httpx"
)

// The browser's refresh token travels in an HttpOnly cookie rather than in a
// response body.
//
// The access token is short-lived and held in memory, so a script injected
// into the page can use it only while the page is open. The refresh token is
// the long-lived credential: in a response body it ends up in storage a script
// can read and send elsewhere, to be redeemed for thirty days. In an HttpOnly
// cookie no script can read it at all.
//
// Opt-in per request, by SessionCookieHeader. Clients that are not browsers --
// the smoke tests, scripts -- keep receiving the token in the body, and a
// browser client still holding a token from before this change can present it
// in the body once to be moved onto the cookie.
//
// CSRF: the cookie is SameSite=Strict, so a cross-site page cannot make the
// browser send it, and it is read only when SessionCookieHeader is present,
// which a cross-origin page cannot add without a CORS preflight this service
// does not grant. Either alone would do.
const (
	// SessionCookieHeader asks for the refresh token in a cookie, and allows
	// it to be read from one.
	SessionCookieHeader = "X-Session-Cookie"
	refreshCookie       = "sp_refresh"
)

func wantsSessionCookie(r *http.Request) bool {
	return r.Header.Get(SessionCookieHeader) == "1"
}

// writeTokens answers a sign-in or refresh. For a cookie client the refresh
// token goes into the cookie and is left out of the body.
func (h *Handler) writeTokens(w http.ResponseWriter, r *http.Request, tokens TokenResponse) {
	if wantsSessionCookie(r) {
		h.setRefreshCookie(w, r, tokens.RefreshToken, int(auth.RefreshTokenTTL.Seconds()))
		tokens.RefreshToken = ""
	}
	httpx.WriteJSON(w, http.StatusOK, tokens)
}

// presentedRefreshToken is the token a refresh or logout acts on: the body's,
// when there is one, otherwise the cookie's for a client that asked for
// cookies.
func presentedRefreshToken(r *http.Request, body string) string {
	if body != "" {
		return body
	}
	if !wantsSessionCookie(r) {
		return ""
	}
	if c, err := r.Cookie(refreshCookie); err == nil {
		return c.Value
	}
	return ""
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter, r *http.Request) {
	h.setRefreshCookie(w, r, "", -1)
}

// setRefreshCookie scopes the cookie to the auth endpoints, as the browser
// sees them, so it is sent to refresh and logout and to nothing else.
//
// Unlike the provider flow, which cannot work without PUBLIC_BASE_URL, password
// sign-in works with it unset -- so this cookie cannot lean on it alone. A
// cookie scoped to the wrong path is never sent back, and every reload would
// sign the user out. The path falls back to the prefix the client says it
// calls this service under (cookiePathFor validates it), and Secure is also
// set for any request that arrived over https.
func (h *Handler) setRefreshCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	secure := h.OAuth.SecureCookies || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	//nolint:gosec // G124: Secure follows the scheme; see setFlowCookie
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    value,
		Path:     h.cookiePath(r),
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		// Strict, unlike the provider flow cookies: nothing that sends this
		// one is a navigation arriving from another site.
		SameSite: http.SameSiteStrictMode,
	})
}

// rotationGrace is how long after a token was rotated presenting it again is
// read as a race, not a theft.
//
// Two tabs share one cookie. When both refresh at once, both send the same
// token; the first rotates it and the second arrives holding a spent token.
// Treated as reuse, that ended every session the user had, for having two
// tabs open. Inside this window the second request is refused without that
// consequence -- its tab retries with the cookie the first one set. A copy
// replayed later than this is still treated as stolen.
const rotationGrace = 30 * time.Second
