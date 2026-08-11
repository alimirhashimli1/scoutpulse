package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/identity-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestAuthIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// testcontainers-go panics rather than returning an error when it cannot
	// resolve a Docker host, so recover and skip instead of failing on
	// machines and CI runners without a daemon.
	//
	// containerStarted bounds that leniency: once the container is up, a panic
	// is a real failure and must not be relabelled as a missing daemon. The
	// football-svc suite had the unbounded version of this block and it turned
	// a genuine connection error into "likely no Docker daemon" on a runner
	// where Docker was working.
	containerStarted := false
	defer func() {
		if r := recover(); r != nil {
			if containerStarted {
				panic(r)
			}
			t.Skipf("Skipping test: testcontainers-go panicked before the container started "+
				"(likely no Docker daemon): %v", r)
		}
	}()

	// Apply every migration in order, so the schema under test is the one the
	// service actually runs against.
	migrations, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	require.NoError(t, err)
	sort.Strings(migrations)
	require.NotEmpty(t, migrations)

	pgContainer, err := postgres.Run(ctx,
		// Matches docker-compose.yml and the football-svc suite. Testing
		// against a different major version than the one deployed is how a
		// version-specific behaviour difference reaches production unnoticed.
		"postgres:16-alpine",
		postgres.WithDatabase("identity_db"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
		postgres.WithInitScripts(migrations...),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Skipf("Skipping test: Docker not available or container failed to start: %v", err)
	}
	containerStarted = true
	defer func() {
		_ = pgContainer.Terminate(ctx)
	}()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sqlx.Open("postgres", connStr)
	require.NoError(t, err)
	defer db.Close()

	h := &Handler{
		UserRepo:    repository.NewPostgresUserRepository(db),
		RefreshRepo: repository.NewPostgresRefreshTokenRepository(db),
	}

	// --- registration ---
	regReq := RegisterRequest{
		Username: "junior_dev",
		Email:    "dev@scoutpulse.com",
		Password: "password123",
	}
	rr := postJSON(t, h.Register, "/api/v1/auth/register", regReq)
	require.Equal(t, http.StatusCreated, rr.Code)

	var userResp model.User
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &userResp))
	assert.Equal(t, regReq.Username, userResp.Username)
	// Self-registration must never produce a privileged account.
	assert.Equal(t, model.UserRole, userResp.Role)

	// --- a duplicate is a conflict, not a 500 ---
	rr = postJSON(t, h.Register, "/api/v1/auth/register", regReq)
	assert.Equal(t, http.StatusConflict, rr.Code)
	// The constraint name must not reach the caller.
	assert.NotContains(t, rr.Body.String(), "users_username_key")

	// --- login ---
	rr = postJSON(t, h.Login, "/api/v1/auth/login",
		LoginRequest{Identifier: regReq.Email, Password: regReq.Password})
	require.Equal(t, http.StatusOK, rr.Code)

	var tokens TokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &tokens))
	require.NotEmpty(t, tokens.AccessToken)
	require.NotEmpty(t, tokens.RefreshToken)

	claims, err := auth.ValidateToken(tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, userResp.ID, claims.UserID)
	assert.Equal(t, string(model.UserRole), claims.Role)
	assert.NotEmpty(t, claims.ID, "the access token needs a jti")

	// --- refresh rotates, and the old token stops working ---
	rr = postJSON(t, h.Refresh, "/api/v1/auth/refresh",
		RefreshRequest{RefreshToken: tokens.RefreshToken})
	require.Equal(t, http.StatusOK, rr.Code)

	var rotated TokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &rotated))
	assert.NotEqual(t, tokens.RefreshToken, rotated.RefreshToken)

	// The token is stored hashed, so a database leak yields no live sessions.
	var storedPlaintext int
	require.NoError(t, db.GetContext(ctx, &storedPlaintext,
		`SELECT COUNT(*) FROM refresh_tokens WHERE token_hash = $1`,
		repository.HashToken(rotated.RefreshToken)))
	assert.Equal(t, 1, storedPlaintext)

	rr = postJSON(t, h.Refresh, "/api/v1/auth/refresh",
		RefreshRequest{RefreshToken: tokens.RefreshToken})
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "a rotated token must not be reusable")

	// Reuse is treated as a leak, so the replacement is revoked too.
	rr = postJSON(t, h.Refresh, "/api/v1/auth/refresh",
		RefreshRequest{RefreshToken: rotated.RefreshToken})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	// --- bad credentials ---
	rr = postJSON(t, h.Login, "/api/v1/auth/login",
		LoginRequest{Identifier: regReq.Email, Password: "wrongpassword"})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	// --- promoting a user ends their existing sessions ---
	rr = postJSON(t, h.Login, "/api/v1/auth/login",
		LoginRequest{Identifier: regReq.Email, Password: regReq.Password})
	require.Equal(t, http.StatusOK, rr.Code)
	var beforePromotion TokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &beforePromotion))

	adminCtx := context.WithValue(ctx, auth.ClaimsContextKey,
		&auth.Claims{UserID: userResp.ID, Role: string(model.AdminRole)})

	body, err := json.Marshal(UpdateRoleRequest{Role: model.EditorRole})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/"+userResp.ID+"/role",
		bytes.NewReader(body)).WithContext(adminCtx)
	req.SetPathValue("id", userResp.ID)
	promoteRR := httptest.NewRecorder()
	h.UpdateRole(promoteRR, req)
	require.Equal(t, http.StatusOK, promoteRR.Code)

	rr = postJSON(t, h.Refresh, "/api/v1/auth/refresh",
		RefreshRequest{RefreshToken: beforePromotion.RefreshToken})
	assert.Equal(t, http.StatusUnauthorized, rr.Code,
		"a role change must invalidate sessions carrying the old role")

	// A fresh login carries the new role.
	rr = postJSON(t, h.Login, "/api/v1/auth/login",
		LoginRequest{Identifier: regReq.Email, Password: regReq.Password})
	require.Equal(t, http.StatusOK, rr.Code)
	var afterPromotion TokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &afterPromotion))

	claims, err = auth.ValidateToken(afterPromotion.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, string(model.EditorRole), claims.Role)
}
