// Package oauth configures the external identity providers users can sign in
// with, and normalises what they return.
//
// Providers disagree about almost everything: the shape of the profile
// endpoint, the field names, whether email verification is reported at all.
// Everything above this package sees a single Profile type, so adding a third
// provider means adding a Provider here and nothing else.
package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/facebook"
	"golang.org/x/oauth2/google"
)

// Name identifies a provider. It is also the value stored in
// user_identities.provider, which has a CHECK constraint listing these.
type Name string

const (
	Google   Name = "google"
	Facebook Name = "facebook"
)

// Profile is the normalised account description a provider returns.
type Profile struct {
	// ProviderUserID is the provider's own immutable id for the account.
	// Accounts are keyed on this, never on the email: an email at a provider
	// can be changed or reassigned, the subject id cannot.
	ProviderUserID string
	Email          string
	// EmailVerified reports whether the provider has confirmed the address.
	//
	// This decides whether an existing local account with the same email may
	// be linked automatically. Treating an unverified address as proof of
	// ownership is an account-takeover route: anyone able to register that
	// address at the provider could claim the matching account here.
	EmailVerified bool
	Name          string
}

// Provider is one configured external identity source.
type Provider struct {
	Name   Name
	Config *oauth2.Config
	// fetchProfile calls the provider's userinfo endpoint and normalises it.
	fetchProfile func(ctx context.Context, client *http.Client) (*Profile, error)
}

// Registry holds the providers that are configured. A provider with no client
// id or secret in the environment is simply absent, so a deployment can enable
// Google without Facebook by setting only Google's variables.
type Registry struct {
	providers map[Name]*Provider
}

// FromEnv builds the registry.
//
// baseURL is this service's externally reachable address, used to construct
// the redirect URI the provider will call back on. It must match what is
// registered in the provider's console exactly, including scheme and port.
func FromEnv(baseURL string) *Registry {
	r := &Registry{providers: make(map[Name]*Provider, 2)}

	if id, secret := os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET"); id != "" && secret != "" {
		r.providers[Google] = &Provider{
			Name: Google,
			Config: &oauth2.Config{
				ClientID:     id,
				ClientSecret: secret,
				RedirectURL:  callbackURL(baseURL, Google),
				Endpoint:     google.Endpoint,
				// openid and email are what this needs; profile supplies a
				// display name. Nothing broader is requested — a consent
				// screen asking for more than the app uses is a reason for
				// users to decline it.
				Scopes: []string{"openid", "email", "profile"},
			},
			fetchProfile: fetchGoogleProfile,
		}
	}

	if id, secret := os.Getenv("FACEBOOK_CLIENT_ID"), os.Getenv("FACEBOOK_CLIENT_SECRET"); id != "" && secret != "" {
		r.providers[Facebook] = &Provider{
			Name: Facebook,
			Config: &oauth2.Config{
				ClientID:     id,
				ClientSecret: secret,
				RedirectURL:  callbackURL(baseURL, Facebook),
				Endpoint:     facebook.Endpoint,
				Scopes:       []string{"email", "public_profile"},
			},
			fetchProfile: fetchFacebookProfile,
		}
	}

	return r
}

func callbackURL(baseURL string, name Name) string {
	return strings.TrimRight(baseURL, "/") + "/api/v1/auth/" + string(name) + "/callback"
}

// Get returns a configured provider.
func (r *Registry) Get(name Name) (*Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Configured lists the providers a client may offer, so a sign-in page can
// show only the buttons that will actually work.
func (r *Registry) Configured() []Name {
	// Ordered rather than map-ranged, so the list a client renders does not
	// reshuffle between requests.
	var names []Name
	for _, n := range []Name{Google, Facebook} {
		if _, ok := r.providers[n]; ok {
			names = append(names, n)
		}
	}
	return names
}

// ParseName validates a provider name from a URL path.
func ParseName(s string) (Name, bool) {
	switch Name(s) {
	case Google:
		return Google, true
	case Facebook:
		return Facebook, true
	default:
		return "", false
	}
}

// Exchange trades the authorization code for a token and fetches the profile.
func (p *Provider) Exchange(ctx context.Context, code, verifier string) (*Profile, error) {
	token, err := p.Config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchanging the authorization code with %s: %w", p.Name, err)
	}

	client := p.Config.Client(ctx, token)
	client.Timeout = 10 * time.Second

	profile, err := p.fetchProfile(ctx, client)
	if err != nil {
		return nil, err
	}
	if profile.ProviderUserID == "" {
		return nil, fmt.Errorf("%s returned no account id", p.Name)
	}
	return profile, nil
}

// fetchGoogleProfile reads Google's userinfo endpoint.
func fetchGoogleProfile(ctx context.Context, client *http.Client) (*Profile, error) {
	var body struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := getJSON(ctx, client, "https://openidconnect.googleapis.com/v1/userinfo", &body); err != nil {
		return nil, fmt.Errorf("reading the Google profile: %w", err)
	}

	return &Profile{
		ProviderUserID: body.Sub,
		Email:          strings.ToLower(strings.TrimSpace(body.Email)),
		EmailVerified:  body.EmailVerified,
		Name:           body.Name,
	}, nil
}

// fetchFacebookProfile reads the Graph API.
func fetchFacebookProfile(ctx context.Context, client *http.Client) (*Profile, error) {
	var body struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := getJSON(ctx, client, "https://graph.facebook.com/v19.0/me?fields=id,name,email", &body); err != nil {
		return nil, fmt.Errorf("reading the Facebook profile: %w", err)
	}

	return &Profile{
		ProviderUserID: body.ID,
		Email:          strings.ToLower(strings.TrimSpace(body.Email)),
		// Facebook does not report verification status on this endpoint. An
		// address it will not vouch for cannot be treated as proof of
		// ownership, so a Facebook sign-in never auto-links to an existing
		// account -- the user is asked to sign in and link deliberately.
		EmailVerified: false,
		Name:          body.Name,
	}, nil
}

func getJSON(ctx context.Context, client *http.Client, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("profile endpoint returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
