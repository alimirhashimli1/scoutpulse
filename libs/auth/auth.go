// Package auth issues and verifies the JSON Web Tokens that authenticate
// callers across ScoutPulse services.
//
// Signing is asymmetric (RS256). The identity service holds the private key
// and is the only process that can mint a token; every other service holds
// only a public key and can verify but not forge. Under the previous shared
// HMAC secret, any service able to check a token could also issue an
// administrator one, which stops being acceptable the moment a third service
// exists.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer identifies tokens minted by this platform.
const Issuer = "scoutpulse-identity"

// Token lifetimes.
//
// The access token is deliberately short. Access tokens are stateless and
// cannot be recalled once issued, so the window in which a leaked one is
// useful is bounded by its expiry rather than by a revocation list. Long-lived
// access is carried by the refresh token instead, which is stored server-side
// and therefore can be revoked.
const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour
)

// signingMethod is the only algorithm this package issues or accepts. Pinning
// it prevents algorithm-confusion attacks during validation.
var signingMethod = jwt.SigningMethodRS256

var (
	// ErrNoSigningKey is returned when a process without the private key
	// attempts to mint a token.
	ErrNoSigningKey = errors.New("auth: no signing key configured; this service cannot issue tokens")
	// ErrNoVerificationKey is returned when no public key is available.
	ErrNoVerificationKey = errors.New("auth: no verification key configured")
)

// Claims is the token payload.
//
// It deliberately carries no team grants. Those used to live here as
// managed_team_ids, which froze an editor's permissions for the token's
// lifetime: a new grant needed a fresh login, and a revocation could not take
// effect at all until expiry. Grants are now resolved per request from the
// service that owns them.
type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	// Username is carried so a consuming service can attribute something to a
	// person without calling back to identity-svc on every write. It is display
	// data, never an authorization input -- UserID and Role are the only fields
	// any decision is made on.
	//
	// Tokens minted before this field existed simply omit it, so a consumer must
	// tolerate it being empty rather than assuming every token carries one.
	Username string `json:"username,omitempty"`
	jwt.RegisteredClaims
}

type contextKey string

const ClaimsContextKey contextKey = "claims"

// GenerateToken mints a short-lived access token.
func GenerateToken(userID, username, role string) (string, error) {
	keysMu.RLock()
	key, kid := signingKey, signingKeyID
	keysMu.RUnlock()

	if key == nil {
		return "", ErrNoSigningKey
	}

	now := time.Now()
	jti, err := randomID()
	if err != nil {
		return "", err
	}

	claims := &Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			Issuer:    Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
		},
	}

	token := jwt.NewWithClaims(signingMethod, claims)
	// The key id lets a verifier pick the right key during a rotation.
	token.Header["kid"] = kid

	return token.SignedString(key)
}

// ValidateToken parses and verifies a token, returning its claims.
func ValidateToken(tokenString string) (*Claims, error) {
	keysMu.RLock()
	haveKeys := len(verificationKeys) > 0
	keysMu.RUnlock()
	if !haveKeys {
		return nil, ErrNoVerificationKey
	}

	parse := func() (*jwt.Token, error) {
		return jwt.ParseWithClaims(
			tokenString,
			&Claims{},
			keyForToken,
			jwt.WithValidMethods([]string{signingMethod.Alg()}),
			jwt.WithIssuer(Issuer),
			jwt.WithExpirationRequired(),
		)
	}

	token, err := parse()
	if errors.Is(err, errUnknownKeyID) {
		// The issuer may have rotated to a key this process has not seen.
		// Refetch the key set once -- rate-limited inside RefreshJWKS, so an
		// invalid kid cannot drive a request per token -- and retry.
		if refreshErr := RefreshJWKS(); refreshErr == nil {
			token, err = parse()
		}
	}
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("auth: invalid token")
	}
	return claims, nil
}

// errUnknownKeyID signals that the token names a key this process does not
// hold, which is the one parse failure worth refetching the key set for.
var errUnknownKeyID = errors.New("auth: unknown key id")

// keyForToken selects the verification key named by the token's kid header.
func keyForToken(token *jwt.Token) (interface{}, error) {
	keysMu.RLock()
	defer keysMu.RUnlock()

	kid, _ := token.Header["kid"].(string)
	if kid != "" {
		if key, ok := verificationKeys[kid]; ok {
			return key, nil
		}
		return nil, fmt.Errorf("%w %q", errUnknownKeyID, kid)
	}

	// A token without a kid is only unambiguous when exactly one key is
	// installed. Guessing among several would mean trying each in turn,
	// which is what a kid exists to avoid.
	if len(verificationKeys) == 1 {
		for _, key := range verificationKeys {
			return key, nil
		}
	}
	return nil, errors.New("auth: token has no key id and multiple keys are configured")
}

// writeUnauthorized emits the same JSON error shape as platform/httpx.
//
// It is duplicated rather than imported because libs/auth is a dependency of
// every service and must not pull the platform module in behind it. The shape
// must stay in step with httpx.ErrorResponse: a client should need one error
// parser, not one for 401 and another for everything else.
func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
		"code":  "unauthorized",
	})
}

// AuthMiddleware validates the bearer token and attaches its claims to the
// request context. Requests without a valid token are rejected.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeUnauthorized(w, "authorization header is required")
			return
		}

		// SplitN rather than Split, and a case-insensitive scheme comparison:
		// RFC 7235 defines the scheme as case-insensitive, and a token itself
		// never contains a space.
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			writeUnauthorized(w, "authorization header must be 'Bearer <token>'")
			return
		}

		claims, err := ValidateToken(strings.TrimSpace(parts[1]))
		if err != nil {
			writeUnauthorized(w, "invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetClaims retrieves claims from the context.
func GetClaims(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(ClaimsContextKey).(*Claims)
	return claims, ok
}

// HasRole reports whether the caller holds the given role.
func (c *Claims) HasRole(role string) bool { return c.Role == role }

// NewRefreshToken returns a cryptographically random opaque token.
//
// A refresh token is not a JWT: it carries no claims and is meaningless
// without the server-side record it points at. That is what makes it
// revocable -- deleting the record invalidates it immediately, which a
// self-contained token can never be.
func NewRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generating refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generating token id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
