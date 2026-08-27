package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/identity-svc/internal/oauth"
	"github.com/scoutpulse/identity-svc/internal/repository"
	libauth "github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
	"golang.org/x/oauth2"
)

// The round trip to a provider and back is bounded: a user who wanders off
// mid-consent should have to start again rather than leave a valid state
// sitting in a cookie.
const (
	oauthFlowTTL  = 10 * time.Minute
	loginCodeTTL  = 60 * time.Second
	stateCookie   = "sp_oauth_state"
	verifierCooki = "sp_oauth_verifier"
	// cookiePath scopes both cookies to the flow that uses them, so they are
	// not sent on every other request to this service.
	//
	// **This is the service's own path, and the browser does not use it.** The
	// gateway publishes this service under /api/identity and strips the prefix
	// before forwarding, so a cookie written with this path alone is scoped to
	// a URL the browser never visits -- and is therefore never sent back. The
	// callback then finds no state and fails as "expired", in well under a
	// millisecond, without ever reaching the provider.
	//
	// cookiePathFor rebuilds the public path from X-Forwarded-Prefix, which the
	// gateway sets for exactly this kind of problem.
	cookiePath = "/api/v1/auth"
)

// cookiePathFor returns the path the *browser* will match against.
//
// Behind the gateway that is "/api/identity" + cookiePath; reached directly it
// is cookiePath unchanged. The header is only trusted to widen the path within
// this origin, never to redirect anything, so a forged value can at most
// mis-scope a short-lived HttpOnly cookie for the client that forged it.
// cookiePath returns the path to scope the flow cookies to, preferring what
// the deployment was configured with over what the request claims.
//
// A configured prefix is known to be right; a header is a hint from whatever
// is in front, and not every proxy sends one.
func (h *Handler) cookiePath(r *http.Request) string {
	if prefix := strings.TrimRight(h.OAuth.PublicPathPrefix, "/"); prefix != "" {
		return prefix + cookiePath
	}
	return cookiePathFor(r)
}

func cookiePathFor(r *http.Request) string {
	prefix := strings.TrimSpace(r.Header.Get("X-Forwarded-Prefix"))
	if prefix == "" {
		return cookiePath
	}
	// A prefix carrying anything but a path is not one we should build on.
	if !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "\r\n;,") {
		return cookiePath
	}
	return strings.TrimRight(prefix, "/") + cookiePath
}

// OAuthDeps are the pieces the provider flow needs beyond the base Handler.
type OAuthDeps struct {
	Providers  *oauth.Registry
	Identities repository.IdentityRepository
	LoginCodes repository.LoginCodeRepository
	// FrontendURL is where a completed sign-in is handed back to. The callback
	// redirects here with a one-time code rather than with tokens.
	FrontendURL string
	// SecureCookies marks the flow cookies Secure. Off for plain-http local
	// development, on everywhere else.
	SecureCookies bool
	// PublicPathPrefix is the path this service is published under, as the
	// browser sees it -- "/api/identity" behind a proxy that mounts it there,
	// empty when it is reached at the root of its own domain.
	//
	// It exists because X-Forwarded-Prefix is not something every proxy sets.
	// Caddy did; Vercel's rewrites do not, and the cookies were then scoped to
	// a path the browser never visits, so every sign-in failed as "expired".
	// Taken from PUBLIC_BASE_URL, which already has to be right for the
	// callback URL to work, so there is no second thing to keep in step.
	PublicPathPrefix string
}

type ExchangeRequest struct {
	Code string `json:"code"`
}

// ListProviders reports which providers are configured, so a sign-in page can
// render only the buttons that will actually work rather than offering a
// Facebook button that 404s.
func (h *Handler) ListProviders(w http.ResponseWriter, r *http.Request) {
	names := h.OAuth.Providers.Configured()
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, string(n))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"providers": out})
}

// StartOAuth sends the user to the provider.
//
//	GET /api/v1/auth/google
func (h *Handler) StartOAuth(w http.ResponseWriter, r *http.Request) {
	provider, err := h.provider(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// state defends the callback against cross-site request forgery: without
	// it, an attacker can make a victim's browser complete a sign-in against
	// the attacker's provider account, silently linking it.
	state, err := libauth.NewRefreshToken()
	if err != nil {
		httpx.WriteError(w, r, apperr.Internal(err))
		return
	}

	// PKCE. Not strictly required for a confidential client that holds a
	// secret, but it costs nothing and removes the value of an intercepted
	// authorization code.
	verifier := oauth2.GenerateVerifier()

	h.setFlowCookie(w, r, stateCookie, state)
	h.setFlowCookie(w, r, verifierCooki, verifier)

	url := provider.Config.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
	)
	http.Redirect(w, r, url, http.StatusFound)
}

// CompleteOAuth receives the provider's callback.
//
// This is a browser navigation, not an API call, so failures redirect back to
// the frontend with an error parameter instead of returning JSON nobody will
// see.
func (h *Handler) CompleteOAuth(w http.ResponseWriter, r *http.Request) {
	provider, err := h.provider(r)
	if err != nil {
		h.failFlow(w, r, "unknown_provider")
		return
	}

	// The provider reports the user declining consent here.
	if e := r.URL.Query().Get("error"); e != "" {
		h.failFlow(w, r, e)
		return
	}

	state, verifier, err := h.readFlowCookies(r)
	h.clearFlowCookies(w, r)
	if err != nil {
		h.failFlow(w, r, "expired")
		return
	}
	// Constant-time is unnecessary here -- state is not a secret being guessed
	// against, it is a value the same browser must echo back.
	if r.URL.Query().Get("state") != state {
		h.failFlow(w, r, "state_mismatch")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.failFlow(w, r, "no_code")
		return
	}

	profile, err := provider.Exchange(r.Context(), code, verifier)
	if err != nil {
		h.failFlow(w, r, "exchange_failed")
		return
	}

	user, err := h.resolveAccount(r, provider.Name, profile)
	if err != nil {
		// The one failure worth naming specifically: an existing account holds
		// this email but the provider will not vouch for it. The frontend can
		// tell the user to sign in with their password and link from settings.
		if apperr.KindOf(err) == apperr.KindConflict {
			h.failFlow(w, r, "email_taken")
			return
		}
		h.failFlow(w, r, "sign_in_failed")
		return
	}

	loginCode, err := h.OAuth.LoginCodes.Issue(r.Context(), user.ID, loginCodeTTL)
	if err != nil {
		h.failFlow(w, r, "sign_in_failed")
		return
	}

	http.Redirect(w, r, h.frontendURL("/auth/callback", url.Values{
		"code": {loginCode},
	}), http.StatusFound)
}

// ExchangeCode turns the one-time code into the normal token pair.
//
//	POST /api/v1/auth/exchange {"code": "..."}
//
// This is why the callback does not put tokens in the redirect: they would end
// up in browser history, access logs and any Referer the next page sends. The
// code is single-use and lives 60 seconds.
func (h *Handler) ExchangeCode(w http.ResponseWriter, r *http.Request) {
	var req ExchangeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if req.Code == "" {
		httpx.WriteError(w, r, apperr.Invalid("code is required"))
		return
	}

	userID, err := h.OAuth.LoginCodes.Redeem(r.Context(), req.Code)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	user, err := h.UserRepo.GetByID(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	tokens, err := h.issueTokens(r, user)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, tokens)
}

// resolveAccount finds, links, or creates the local account for a provider
// profile. This is where the account-linking policy lives.
func (h *Handler) resolveAccount(r *http.Request, provider oauth.Name, profile *oauth.Profile) (*model.User, error) {
	ctx := r.Context()

	// 1. Already linked. The common path, and the only one keyed on the
	//    provider's own id rather than on an email.
	identity, err := h.OAuth.Identities.GetByProviderAccount(ctx, string(provider), profile.ProviderUserID)
	if err == nil {
		_ = h.OAuth.Identities.TouchLogin(ctx, identity.ID)
		return h.UserRepo.GetByID(ctx, identity.UserID)
	}
	if apperr.KindOf(err) != apperr.KindNotFound {
		return nil, err
	}

	// 2. An account already holds this email.
	if profile.Email != "" {
		existing, err := h.UserRepo.GetByIdentifier(ctx, profile.Email)
		switch {
		case err == nil:
			// Link only when the provider has actually verified the address.
			// Otherwise anyone able to register that email at the provider
			// could claim this account -- which is the whole reason the
			// verified flag is checked rather than trusted implicitly.
			if !profile.EmailVerified {
				return nil, apperr.Conflict(
					"an account already uses this email; sign in with your password and link the provider from settings")
			}
			if linkErr := h.linkIdentity(ctx, existing.ID, provider, profile); linkErr != nil {
				return nil, linkErr
			}
			return existing, nil

		case apperr.KindOf(err) == apperr.KindNotFound:
			// Fall through to creating a new account.

		default:
			return nil, err
		}
	}

	// 3. Nobody here yet. Create the account.
	return h.createFromProvider(r, provider, profile)
}

func (h *Handler) linkIdentity(ctx context.Context, userID string, provider oauth.Name, profile *oauth.Profile) error {
	var email *string
	if profile.Email != "" {
		e := profile.Email
		email = &e
	}
	return h.OAuth.Identities.Link(ctx, &model.Identity{
		UserID:         userID,
		Provider:       string(provider),
		ProviderUserID: profile.ProviderUserID,
		Email:          email,
	})
}

func (h *Handler) createFromProvider(r *http.Request, provider oauth.Name, profile *oauth.Profile) (*model.User, error) {
	ctx := r.Context()

	username, err := h.availableUsername(r, profile)
	if err != nil {
		return nil, err
	}

	email := profile.Email
	if email == "" {
		// Facebook can withhold the email. A synthetic address keeps the NOT
		// NULL and UNIQUE constraints satisfied without colliding, and is
		// obviously not a real inbox to anyone reading the table.
		email = fmt.Sprintf("%s-%s@users.noreply.scoutpulse", provider, profile.ProviderUserID)
	}

	user := model.User{
		ID:       uuid.New().String(),
		Username: username,
		Email:    email,
		// No password. The column is nullable since migration 000003 precisely
		// so this does not need a placeholder that could later be compared
		// against.
		PasswordHash: "",
		// A provider sign-in grants no more than self-registration does.
		Role: model.UserRole,
		// The provider has already proven the address, so there is nothing for
		// our own confirmation email to add -- and sending one to somebody who
		// signed in with Google would be a step they cannot make sense of.
		// Facebook never reports verification, so those accounts stay unverified,
		// which is correct: it is the same reason they are not auto-linked.
		EmailVerified: profile.EmailVerified,
	}

	if err := h.UserRepo.Create(ctx, &user); err != nil {
		return nil, err
	}
	if err := h.linkIdentity(ctx, user.ID, provider, profile); err != nil {
		return nil, err
	}
	return &user, nil
}

// availableUsername derives a username that is not already taken.
//
// Providers supply display names, which collide constantly -- two people
// called "Ali" would otherwise make the second sign-in fail with a unique
// violation they can do nothing about.
func (h *Handler) availableUsername(r *http.Request, profile *oauth.Profile) (string, error) {
	base := sanitiseUsername(profile.Name)
	if base == "" && profile.Email != "" {
		base, _, _ = strings.Cut(profile.Email, "@")
		base = sanitiseUsername(base)
	}
	if base == "" {
		base = "user"
	}

	candidate := base
	for attempt := 0; attempt < 5; attempt++ {
		_, err := h.UserRepo.GetByIdentifier(r.Context(), candidate)
		if apperr.KindOf(err) == apperr.KindNotFound {
			return candidate, nil
		}
		if err != nil && apperr.KindOf(err) != apperr.KindNotFound {
			return "", err
		}
		// Taken: add a short random suffix rather than a counter, which would
		// otherwise probe how many similar names exist.
		suffix, sErr := libauth.NewRefreshToken()
		if sErr != nil {
			return "", apperr.Internal(sErr)
		}
		candidate = base + "-" + strings.ToLower(suffix[:6])
	}
	return "", apperr.Wrap(apperr.KindInternal, "could not allocate a username", errors.New("exhausted attempts"))
}

func sanitiseUsername(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '_' || r == '-':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

// --- linked providers -------------------------------------------------

// ListIdentities shows which providers the caller has linked.
func (h *Handler) ListIdentities(w http.ResponseWriter, r *http.Request) {
	claims, ok := libauth.GetClaims(r.Context())
	if !ok {
		httpx.WriteError(w, r, apperr.Unauthorized("authentication required"))
		return
	}

	identities, err := h.OAuth.Identities.ListForUser(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if identities == nil {
		identities = []model.Identity{}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"identities": identities})
}

// UnlinkIdentity detaches a provider from the caller's account.
//
// Refused when it would leave the account with no way to sign in at all --
// no password and no other provider. Locking somebody out of their own
// account through a settings toggle is not a recoverable mistake here, since
// there is no password reset.
func (h *Handler) UnlinkIdentity(w http.ResponseWriter, r *http.Request) {
	claims, ok := libauth.GetClaims(r.Context())
	if !ok {
		httpx.WriteError(w, r, apperr.Unauthorized("authentication required"))
		return
	}

	provider, ok := oauth.ParseName(r.PathValue("provider"))
	if !ok {
		httpx.WriteError(w, r, apperr.Invalid("unknown provider"))
		return
	}

	user, err := h.UserRepo.GetByID(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	identities, err := h.OAuth.Identities.ListForUser(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if user.PasswordHash == "" && len(identities) <= 1 {
		httpx.WriteError(w, r, apperr.Invalid(
			"this is the only way to sign in to this account; set a password before unlinking it"))
		return
	}

	if err := h.OAuth.Identities.Unlink(r.Context(), claims.UserID, string(provider)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ----------------------------------------------------------

func (h *Handler) provider(r *http.Request) (*oauth.Provider, error) {
	name, ok := oauth.ParseName(r.PathValue("provider"))
	if !ok {
		return nil, apperr.NotFound("unknown provider")
	}
	p, ok := h.OAuth.Providers.Get(name)
	if !ok {
		// Configured providers are advertised by ListProviders, so this means
		// the deployment has no credentials for one a client asked for.
		return nil, apperr.NotFound("that provider is not enabled")
	}
	return p, nil
}

func (h *Handler) setFlowCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	path := h.cookiePath(r)

	// gosec G124 wants Secure set to a literal true. It is set from
	// configuration instead, and deliberately: a Secure cookie is silently
	// dropped over plain http, so hardcoding it would break local development
	// in a way that presents as the state check failing rather than as a
	// cookie problem. HttpOnly and SameSite are unconditional, and Secure is
	// on for every deployment reached over https.
	//
	//nolint:gosec // G124: Secure is set from PUBLIC_BASE_URL's scheme
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   int(oauthFlowTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.OAuth.SecureCookies,
		// Lax, not Strict: the callback arrives as a top-level navigation from
		// the provider's domain, and Strict would withhold the cookie exactly
		// then, breaking every sign-in.
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) readFlowCookies(r *http.Request) (state, verifier string, err error) {
	s, err := r.Cookie(stateCookie)
	if err != nil {
		return "", "", err
	}
	v, err := r.Cookie(verifierCooki)
	if err != nil {
		return "", "", err
	}
	return s.Value, v.Value, nil
}

func (h *Handler) clearFlowCookies(w http.ResponseWriter, r *http.Request) {
	path := h.cookiePath(r)

	for _, name := range []string{stateCookie, verifierCooki} {
		//nolint:gosec // G124: same as setFlowCookie — Secure follows the scheme
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     path,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   h.OAuth.SecureCookies,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// failFlow returns the browser to the frontend with a reason it can render.
// The reasons are deliberately coarse: a precise one would tell an attacker
// which stage of a forged flow they reached.
// failFlow ends the flow with a reason the frontend can act on.
//
// The reason is deliberately coarse in the redirect -- it reaches the user as a
// URL parameter -- but it is logged in full here. Without that, every failure
// looked identical from outside and identical in the log: a 302 with no error
// line, which is what made a mis-scoped cookie take a code read to find.
func (h *Handler) failFlow(w http.ResponseWriter, r *http.Request, reason string) {
	if h.Log != nil {
		h.Log.Warn("external sign-in failed",
			"reason", reason,
			"provider", r.PathValue("provider"),
			"cookie_path", h.cookiePath(r),
			"had_state_cookie", hasCookie(r, stateCookie),
			"forwarded_prefix", r.Header.Get("X-Forwarded-Prefix"))
	}
	h.clearFlowCookies(w, r)
	http.Redirect(w, r, h.frontendURL("/auth/callback", url.Values{
		"error": {reason},
	}), http.StatusFound)
}

func (h *Handler) frontendURL(path string, params url.Values) string {
	base := strings.TrimRight(h.OAuth.FrontendURL, "/")
	return base + path + "?" + params.Encode()
}

func hasCookie(r *http.Request, name string) bool {
	_, err := r.Cookie(name)
	return err == nil
}
