package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/scoutpulse/identity-svc/internal/captcha"
	"github.com/scoutpulse/identity-svc/internal/mailer"
	"github.com/scoutpulse/identity-svc/internal/validate"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
	"github.com/scoutpulse/libs/platform/server"
)

// VerificationTTL is how long a link stays good.
//
// Long enough to survive an inbox someone checks in the evening, short enough
// that a message sitting in an abandoned mailbox stops being a way in.
const VerificationTTL = 24 * time.Hour

// VerifyEmailRequest carries the token from the link.
//
// Sent as a POST body rather than read from the query string, for the same
// reason the OAuth flow exchanges a code instead of putting tokens in the URL:
// query strings land in browser history, server access logs, and the Referer
// header of whatever the page loads next.
type VerifyEmailRequest struct {
	Token string `json:"token"`
}

type ResendVerificationRequest struct {
	Email   string `json:"email"`
	Captcha string `json:"captcha_token"`
}

// sendVerificationEmail issues a token and mails the link.
//
// Errors are returned rather than swallowed, but callers decide what to do
// with them -- registration should not fail because the mail server hiccuped,
// since the account exists by then and a resend can fix it.
func (h *Handler) sendVerificationEmail(r *http.Request, userID, email string) error {
	if h.Verification == nil || h.Mailer == nil {
		return nil
	}

	token, err := h.Verification.Issue(r.Context(), userID, VerificationTTL)
	if err != nil {
		return err
	}

	// The link points at the frontend, not this API. The page there posts the
	// token back, which keeps the token out of a GET and lets the app show a
	// result rather than raw JSON.
	link := fmt.Sprintf("%s/verify-email?token=%s",
		strings.TrimRight(h.OAuth.FrontendURL, "/"), url.QueryEscape(token))

	return h.Mailer.Send(r.Context(), mailer.Message{
		To:      email,
		Subject: "Confirm your ScoutPulse address",
		Body: "Someone created a ScoutPulse account with this address.\r\n\r\n" +
			"Confirm it by opening this link:\r\n\r\n" + link + "\r\n\r\n" +
			"The link works once and expires in 24 hours.\r\n\r\n" +
			"If this was not you, ignore this message -- the account cannot be " +
			"used until the address is confirmed.\r\n",
	})
}

// VerifyEmail consumes a token and marks the address proven.
func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	if h.Verification == nil {
		httpx.WriteError(w, r, apperr.Invalid("email verification is not enabled"))
		return
	}

	var req VerifyEmailRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		httpx.WriteError(w, r, apperr.Invalid("token is required"))
		return
	}

	userID, err := h.Verification.Redeem(r.Context(), req.Token)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err := h.UserRepo.MarkEmailVerified(r.Context(), userID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"verified": true})
}

// ResendVerification issues a fresh link.
//
// Answers identically whether or not the address exists. Reporting "no such
// account" here would turn this endpoint into a way to test which addresses
// are registered -- the same reasoning that makes login report one message for
// a wrong password and an unknown user.
func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req ResendVerificationRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if h.Captcha != nil {
		if err := h.Captcha.Verify(r.Context(), req.Captcha, server.ClientIP(r)); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}

	// Always the same response, whatever happens below.
	accepted := func() {
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
			"message": "If that address needs confirming, a link is on its way.",
		})
	}

	email, err := validate.Email(req.Email)
	if err != nil {
		accepted()
		return
	}

	user, err := h.UserRepo.GetByIdentifier(r.Context(), email)
	if err != nil || user.EmailVerified || user.PasswordHash == "" {
		// Unknown address, already verified, or an account that signs in
		// through a provider and has no address of ours to confirm.
		accepted()
		return
	}

	if err := h.sendVerificationEmail(r, user.ID, user.Email); err != nil {
		// Logged by the mailer. The caller still gets the neutral response:
		// telling them delivery failed would confirm the address exists.
		h.logVerificationFailure(user.ID, err)
	}

	accepted()
}

func (h *Handler) logVerificationFailure(userID string, err error) {
	if h.Log == nil {
		return
	}
	h.Log.Error("could not send verification email", "user_id", userID, "error", err)
}

// AuthConfig tells the frontend how to render the sign-in and sign-up pages:
// which external providers exist, whether a challenge is required and with
// which public key, and whether a new account must confirm its address.
//
// One call rather than three, and public — every value in it is something the
// browser needs before anyone has authenticated. The captcha *secret* is not
// here; only the site key, which is designed to be published.
func (h *Handler) AuthConfig(w http.ResponseWriter, r *http.Request) {
	names := h.OAuth.Providers.Configured()
	providers := make([]string, 0, len(names))
	for _, name := range names {
		providers = append(providers, string(name))
	}
	sort.Strings(providers)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"providers":             providers,
		"captcha":               captcha.Describe(h.CaptchaKind, h.CaptchaSite),
		"verification_required": h.Verification != nil,
	})
}
