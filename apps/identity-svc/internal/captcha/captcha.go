// Package captcha verifies a human-challenge token server-side.
//
// The only part that matters for security is that verification happens *here*.
// The widget in the browser proves nothing on its own -- a client that wants to
// skip it simply does not render it, and posts to the API directly. What stops
// automated registration is this service refusing a request whose token the
// provider does not vouch for.
package captcha

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/scoutpulse/libs/platform/apperr"
)

// Verifier checks a token submitted with a form.
type Verifier interface {
	// Verify returns nil when the token is good. remoteIP may be empty.
	Verify(ctx context.Context, token, remoteIP string) error
	// Enabled reports whether a challenge is actually required, so handlers
	// and the frontend can agree on whether to ask for one.
	Enabled() bool
}

// Provider endpoints. Both take the same form fields and return the same
// JSON shape, which is why one implementation covers them.
const (
	recaptchaVerifyURL = "https://www.google.com/recaptcha/api/siteverify"
	turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
)

type Config struct {
	// Provider is "recaptcha" or "turnstile". Empty disables the check.
	Provider string
	Secret   string
	// MinScore applies to reCAPTCHA v3 only, which returns a 0..1 likelihood
	// rather than a pass/fail. Ignored when zero.
	MinScore float64
}

// New returns a verifier, or a disabled one when no secret is configured.
//
// Disabled rather than fatal, matching how absent OAuth credentials are
// handled: a developer running this locally should not have to register with
// Cloudflare or Google before they can create an account.
func New(cfg Config, log *slog.Logger) Verifier {
	if cfg.Secret == "" || cfg.Provider == "" {
		log.Warn("no CAPTCHA_SECRET set, sign-up and sign-in are not challenge-protected")
		return disabled{}
	}

	var endpoint string
	switch strings.ToLower(cfg.Provider) {
	case "recaptcha":
		endpoint = recaptchaVerifyURL
	case "turnstile":
		endpoint = turnstileVerifyURL
	default:
		log.Warn("unknown CAPTCHA_PROVIDER, challenge disabled", "provider", cfg.Provider)
		return disabled{}
	}

	log.Info("captcha enabled", "provider", cfg.Provider)
	return &remote{
		cfg:      cfg,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

type disabled struct{}

func (disabled) Verify(context.Context, string, string) error { return nil }
func (disabled) Enabled() bool                                { return false }

type remote struct {
	cfg      Config
	endpoint string
	client   *http.Client
}

func (r *remote) Enabled() bool { return true }

type verifyResponse struct {
	Success    bool     `json:"success"`
	Score      float64  `json:"score"`
	ErrorCodes []string `json:"error-codes"`
}

func (r *remote) Verify(ctx context.Context, token, remoteIP string) error {
	if strings.TrimSpace(token) == "" {
		return apperr.Invalid("please complete the challenge")
	}

	form := url.Values{"secret": {r.cfg.Secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return apperr.Internal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := r.client.Do(req)
	if err != nil {
		// The provider being unreachable is our outage, not the visitor's
		// fault -- but failing open would make the check bypassable by
		// blocking one hostname, so it fails closed and says so plainly.
		return apperr.Wrap(apperr.KindInternal, "could not verify the challenge, please try again", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body verifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return apperr.Wrap(apperr.KindInternal, "could not verify the challenge, please try again", err)
	}

	if !body.Success {
		// The provider's error codes describe *our* configuration as often as
		// the visitor's token -- an invalid secret reports here. Logged detail
		// belongs in the response only as a generic message.
		return apperr.Invalid("challenge failed, please try again")
	}
	if r.cfg.MinScore > 0 && body.Score > 0 && body.Score < r.cfg.MinScore {
		return apperr.Invalid("challenge failed, please try again")
	}

	return nil
}

// Describe is what the frontend needs in order to render the widget: which
// provider, and the public site key. The secret never leaves this service.
func Describe(provider, siteKey string) map[string]any {
	enabled := provider != "" && siteKey != ""
	return map[string]any{
		"enabled":  enabled,
		"provider": provider,
		"site_key": siteKey,
	}
}
