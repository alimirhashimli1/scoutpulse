package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"time"
)

// isLoopbackOrInternal reports whether host is one a local or in-cluster stack
// would use, where requiring https would mean requiring certificates for
// service-to-service traffic that never leaves the network.
func isLoopbackOrInternal(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".svc") ||
		strings.HasSuffix(host, ".cluster.local") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	// A bare single-label name is a compose or Kubernetes service name
	// ("identity-svc"), which is not routable off the network.
	return host != "" && !strings.Contains(host, ".")
}

// JWKSPath is the conventional location of a JWKS document.
const JWKSPath = "/.well-known/jwks.json"

// JWK is one key in a JWKS document, in the subset of RFC 7517 needed for RSA
// signature verification.
type JWK struct {
	Kty string `json:"kty"` // key type, always "RSA" here
	Use string `json:"use"` // "sig"
	Alg string `json:"alg"` // "RS256"
	Kid string `json:"kid"`
	N   string `json:"n"` // modulus, base64url
	E   string `json:"e"` // exponent, base64url
}

// JWKS is a set of public keys.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// PublicJWKS returns the verification keys this process holds, for publishing.
//
// Only public material is ever included: a JWKS document is meant to be
// world-readable, and the private key never leaves the issuer.
func PublicJWKS() JWKS {
	keysMu.RLock()
	defer keysMu.RUnlock()

	set := JWKS{Keys: make([]JWK, 0, len(verificationKeys))}
	for kid, key := range verificationKeys {
		set.Keys = append(set.Keys, JWK{
			Kty: "RSA",
			Use: "sig",
			Alg: signingMethod.Alg(),
			Kid: kid,
			N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		})
	}
	return set
}

// JWKSHandler serves the public keys so other services can verify tokens
// without being given the private key, and so a rotation propagates without
// redeploying every consumer.
func JWKSHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Cacheable: keys change rarely, and consumers refetch on an
		// unrecognised key id anyway.
		w.Header().Set("Cache-Control", "public, max-age=300")
		_ = json.NewEncoder(w).Encode(PublicJWKS())
	}
}

// jwksClient fetches remote key sets.
var jwksClient = &http.Client{Timeout: 10 * time.Second}

// jwksState tracks the source to refetch from, and rate-limits refetches so an
// unknown key id cannot be used to drive a request per token.
var jwksState struct {
	mu            sync.Mutex
	url           string
	lastRefreshed time.Time
}

// MinJWKSRefreshInterval is the shortest gap between two refetches triggered by
// an unrecognised key id.
const MinJWKSRefreshInterval = 30 * time.Second

// JWKSRefreshInterval is how often a background refresh re-reads the document.
const JWKSRefreshInterval = 5 * time.Minute

// LoadJWKS fetches a JWKS document and installs every key it contains. The URL
// is retained so the key set can be refreshed later without reconfiguration.
func LoadJWKS(url string) error {
	if err := loadJWKSOnce(url); err != nil {
		return err
	}

	jwksState.mu.Lock()
	jwksState.url = url
	jwksState.lastRefreshed = time.Now()
	jwksState.mu.Unlock()
	return nil
}

// RefreshJWKS re-reads the configured JWKS document. It is a no-op when no
// JWKS URL is in use, or when a refresh happened within the last
// MinJWKSRefreshInterval.
//
// This is what makes a key rotation propagate: a token signed with a key this
// process has not seen triggers one refetch, and the new key is installed
// without a redeploy.
func RefreshJWKS() error {
	jwksState.mu.Lock()
	url := jwksState.url
	if url == "" || time.Since(jwksState.lastRefreshed) < MinJWKSRefreshInterval {
		jwksState.mu.Unlock()
		return nil
	}
	jwksState.lastRefreshed = time.Now()
	jwksState.mu.Unlock()

	return loadJWKSOnce(url)
}

// StartJWKSRefresh polls the JWKS document until ctx is cancelled, so a
// rotation is picked up even by a service that is not currently seeing tokens
// signed with the new key.
func StartJWKSRefresh(ctx context.Context, logger *slog.Logger) {
	jwksState.mu.Lock()
	url := jwksState.url
	jwksState.mu.Unlock()
	if url == "" {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	go func() {
		ticker := time.NewTicker(JWKSRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				jwksState.mu.Lock()
				jwksState.lastRefreshed = time.Now()
				jwksState.mu.Unlock()
				if err := loadJWKSOnce(url); err != nil {
					// A failed refresh is not fatal: the keys already
					// installed stay valid and serving continues.
					logger.Warn("jwks refresh failed", "url", url, "error", err)
				}
			}
		}
	}()
}

func loadJWKSOnce(rawURL string) error {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("auth: invalid JWKS URL %q: %w", rawURL, err)
	}
	// Over plain http the key set can be substituted in transit, and these
	// keys decide which tokens this service trusts. Loopback is allowed so a
	// local stack works without certificates.
	if parsed.Scheme != "https" && !isLoopbackOrInternal(parsed.Hostname()) {
		return fmt.Errorf("auth: JWKS URL must use https (got %q)", parsed.Scheme)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("auth: building JWKS request: %w", err)
	}

	resp, err := jwksClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth: fetching JWKS from %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	url := rawURL
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth: JWKS endpoint %s returned %s", url, resp.Status)
	}

	var set JWKS
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("auth: decoding JWKS from %s: %w", url, err)
	}
	if len(set.Keys) == 0 {
		return fmt.Errorf("auth: JWKS at %s contains no keys", url)
	}

	keysMu.Lock()
	defer keysMu.Unlock()
	for _, jwk := range set.Keys {
		key, err := jwk.toRSAPublicKey()
		if err != nil {
			// Skip keys this version cannot use rather than rejecting the
			// whole document; an issuer may publish types we do not support.
			continue
		}
		verificationKeys[jwk.Kid] = key
	}

	if len(verificationKeys) == 0 {
		return fmt.Errorf("auth: JWKS at %s contained no usable RSA keys", url)
	}
	return nil
}

func (j JWK) toRSAPublicKey() (*rsa.PublicKey, error) {
	if j.Kty != "RSA" {
		return nil, fmt.Errorf("auth: unsupported key type %q", j.Kty)
	}

	n, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		return nil, fmt.Errorf("auth: decoding modulus: %w", err)
	}
	e, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		return nil, fmt.Errorf("auth: decoding exponent: %w", err)
	}

	exponent := new(big.Int).SetBytes(e)
	if !exponent.IsInt64() || exponent.Int64() > int64(^uint32(0)) {
		return nil, fmt.Errorf("auth: exponent out of range")
	}

	key := &rsa.PublicKey{
		N: new(big.Int).SetBytes(n),
		E: int(exponent.Int64()),
	}
	// A published key set is outside this service's control, so the same size
	// floor applies here as to a configured key.
	if err := checkKeySize(key); err != nil {
		return nil, err
	}
	return key, nil
}
