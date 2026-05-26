package authmiddleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = []byte("scoutpulse_secret_key") // In production, use env variable

type Claims struct {
	UserID         string   `json:"user_id"`
	Role           string   `json:"role"`
	ManagedTeamIDs []string `json:"managed_team_ids"`
	jwt.RegisteredClaims
}

type contextKey string

const ClaimsContextKey contextKey = "claims"

// ValidateToken parses and validates a JWT string.
func ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// AuthMiddleware is a reusable middleware for validating JWTs.
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

		// Add claims to context
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
