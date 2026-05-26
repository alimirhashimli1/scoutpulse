package authmiddleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func generateTestToken(userID, role string, teams []string) (string, error) {
	claims := &Claims{
		UserID:         userID,
		Role:           role,
		ManagedTeamIDs: teams,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func TestAuthMiddleware(t *testing.T) {
	t.Run("Valid Token", func(t *testing.T) {
		token, _ := generateTestToken("123", "editor", []string{"teamA"})
		
		req, _ := http.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := GetClaims(r.Context())
			assert.True(t, ok)
			assert.Equal(t, "123", claims.UserID)
			assert.Equal(t, "editor", claims.Role)
			assert.Contains(t, claims.ManagedTeamIDs, "teamA")
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Missing Header", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Invalid Format", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "InvalidToken")
		rr := httptest.NewRecorder()

		handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Expired Token", func(t *testing.T) {
		claims := &Claims{
			UserID: "123",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			},
		}
		token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)

		req, _ := http.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestHasTeamPermission(t *testing.T) {
	claims := &Claims{
		Role:           "editor",
		ManagedTeamIDs: []string{"team1", "team2"},
	}

	assert.True(t, claims.HasTeamPermission("team1"))
	assert.True(t, claims.HasTeamPermission("team2"))
	assert.False(t, claims.HasTeamPermission("team3"))

	adminClaims := &Claims{
		Role: "admin",
	}
	assert.True(t, adminClaims.HasTeamPermission("anything"))
}
