package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SecretEnvVar is the environment variable holding the JWT signing key.
const SecretEnvVar = "JWT_SECRET"

// MinSecretLength is the shortest signing key accepted. HS256 keys shorter
// than the hash output add no security over a 32-byte key.
const MinSecretLength = 32

// TokenTTL is how long an issued token stays valid.
const TokenTTL = 24 * time.Hour

// signingMethod is the only algorithm this package will issue or accept.
// Pinning it prevents algorithm-confusion attacks on validation.
var signingMethod = jwt.SigningMethodHS256

var (
	// ErrSecretNotConfigured is returned when a token operation is attempted
	// before a signing key has been installed.
	ErrSecretNotConfigured = errors.New("auth: JWT signing key is not configured")

	secretMu  sync.RWMutex
	jwtSecret []byte
)

type Claims struct {
	UserID         string   `json:"user_id"`
	Role           string   `json:"role"`
	ManagedTeamIDs []string `json:"managed_team_ids"`
	jwt.RegisteredClaims
}

type contextKey string

const ClaimsContextKey contextKey = "claims"

// SetSecret installs the JWT signing key. It is safe for concurrent use, but
// is normally called once during service startup.
func SetSecret(secret []byte) error {
	if len(secret) < MinSecretLength {
		return fmt.Errorf("auth: signing key must be at least %d bytes, got %d", MinSecretLength, len(secret))
	}

	secretMu.Lock()
	defer secretMu.Unlock()
	jwtSecret = append([]byte(nil), secret...)
	return nil
}

// LoadSecretFromEnv installs the signing key from JWT_SECRET. Services should
// call this at startup and treat a failure as fatal: without a key, every
// token operation fails closed.
func LoadSecretFromEnv() error {
	secret, ok := os.LookupEnv(SecretEnvVar)
	if !ok || secret == "" {
		return fmt.Errorf("auth: %s is not set", SecretEnvVar)
	}
	return SetSecret([]byte(secret))
}

func currentSecret() ([]byte, error) {
	secretMu.RLock()
	defer secretMu.RUnlock()
	if len(jwtSecret) == 0 {
		return nil, ErrSecretNotConfigured
	}
	return jwtSecret, nil
}

// GenerateToken creates a new JWT for a user.
func GenerateToken(userID string, role string, managedTeamIDs []string) (string, error) {
	secret, err := currentSecret()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := &Claims{
		UserID:         userID,
		Role:           role,
		ManagedTeamIDs: managedTeamIDs,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    "scoutpulse-identity",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
		},
	}

	return jwt.NewWithClaims(signingMethod, claims).SignedString(secret)
}

// ValidateToken parses and validates a JWT string.
func ValidateToken(tokenString string) (*Claims, error) {
	secret, err := currentSecret()
	if err != nil {
		return nil, err
	}

	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(*jwt.Token) (interface{}, error) { return secret, nil },
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
	)
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// AuthMiddleware validates the bearer token and attaches its claims to the
// request context. Requests without a valid token are rejected.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header is required", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		claims, err := ValidateToken(parts[1])
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
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

// HasRole checks if the user has a specific role.
func (c *Claims) HasRole(role string) bool {
	return c.Role == role
}

// HasTeamPermission checks if the user has permission for a specific team.
func (c *Claims) HasTeamPermission(teamID string) bool {
	if c.Role == "admin" {
		return true
	}
	for _, id := range c.ManagedTeamIDs {
		if id == teamID {
			return true
		}
	}
	return false
}
