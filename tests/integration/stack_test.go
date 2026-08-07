// Package integration exercises the full docker-compose stack: real
// containers, real migrations, real HTTP.
//
// These tests are slow and require a Docker daemon. They are skipped under
// -short.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/compose"
)

// The service's domain types live under internal/, which another module
// cannot import. That is the right constraint for a black-box stack test:
// these mirror only the wire fields the test actually asserts on, so a change
// to the service's internals does not silently change what is being verified.

type league struct {
	ID      string `json:"id" db:"id"`
	Name    string `json:"name" db:"name"`
	Country string `json:"country" db:"country"`
}

type team struct {
	ID       string  `json:"id" db:"id"`
	LeagueID *string `json:"league_id" db:"league_id"`
	Name     string  `json:"name" db:"name"`
}

type player struct {
	ID          string  `json:"id" db:"id"`
	TeamID      *string `json:"team_id" db:"team_id"`
	Name        string  `json:"name" db:"name"`
	Position    string  `json:"position" db:"position"`
	MarketValue float64 `json:"market_value" db:"market_value"`
}

const (
	// Must match the compose service names in docker-compose.yml.
	footballService   = "football-svc"
	footballDBService = "postgres-football"

	// Ports the containers listen on internally.
	footballServicePort = "8081"
	postgresPort        = "5432"

	// The stack needs a signing key; both services must agree on it.
	testJWTSecret = "integration-test-signing-key-at-least-32-chars"

	stackTimeout = 5 * time.Minute
)

// startStack brings up the compose stack and registers its teardown.
func startStack(ctx context.Context, t *testing.T) compose.ComposeStack {
	t.Helper()

	composeFile := filepath.Join("..", "..", "docker-compose.yml")
	stack, err := compose.NewDockerCompose(composeFile)
	if err != nil {
		t.Skipf("Skipping: docker compose unavailable: %v", err)
	}

	err = stack.
		WithEnv(map[string]string{
			"JWT_SECRET":           testJWTSecret,
			"DB_USER":              "user",
			"DB_PASSWORD":          "password",
			"CORS_ALLOWED_ORIGINS": "*",
		}).
		Up(ctx, compose.Wait(true))
	if err != nil {
		t.Skipf("Skipping: could not start the stack: %v", err)
	}

	t.Cleanup(func() {
		downCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = stack.Down(downCtx, compose.RemoveOrphans(true), compose.RemoveVolumes(true))
	})

	return stack
}

// endpoint returns the host-reachable base URL for a compose service.
func endpoint(ctx context.Context, t *testing.T, stack compose.ComposeStack, service, port string) (string, string) {
	t.Helper()

	container, err := stack.ServiceContainer(ctx, service)
	require.NoError(t, err, "resolving container for %s", service)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	mapped, err := container.MappedPort(ctx, nat.Port(port+"/tcp"))
	require.NoError(t, err)

	return host, mapped.Port()
}

// postJSON sends an authenticated POST and decodes the created resource.
func postJSON(ctx context.Context, t *testing.T, client *http.Client, url, token string, payload, out any) {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "POST %s", url)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
}

// TestTeamDeletionNullsPlayerTeam verifies the ON DELETE SET NULL behaviour
// the schema relies on: deleting a club must not delete its players, it must
// turn them into free agents.
func TestTeamDeletionNullsPlayerTeam(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stack test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), stackTimeout)
	defer cancel()

	stack := startStack(ctx, t)

	svcHost, svcPort := endpoint(ctx, t, stack, footballService, footballServicePort)
	baseURL := fmt.Sprintf("http://%s:%s/api/v1", svcHost, svcPort)

	dbHost, dbPort := endpoint(ctx, t, stack, footballDBService, postgresPort)
	footballDB, err := db.Connect(db.Config{
		Host:     dbHost,
		Port:     dbPort,
		User:     "user",
		Password: "password",
		DBName:   "football_db",
	})
	require.NoError(t, err)
	defer footballDB.Close()

	require.NoError(t, auth.SetSecret([]byte(testJWTSecret)))
	adminToken, err := auth.GenerateToken("test-admin", "admin", nil)
	require.NoError(t, err)

	client := &http.Client{Timeout: 10 * time.Second}

	// IDs are generated by the database, so each response is read back rather
	// than assumed.
	var createdLeague league
	postJSON(ctx, t, client, baseURL+"/leagues", adminToken,
		league{Name: "Test League", Country: "Test Country"}, &createdLeague)
	require.NotEmpty(t, createdLeague.ID)

	var createdTeam team
	postJSON(ctx, t, client, baseURL+"/teams", adminToken,
		team{LeagueID: &createdLeague.ID, Name: "Test Team"}, &createdTeam)
	require.NotEmpty(t, createdTeam.ID)

	var createdPlayer player
	postJSON(ctx, t, client, baseURL+"/players", adminToken,
		player{TeamID: &createdTeam.ID, Name: "Test Player", Position: "Forward", MarketValue: 100},
		&createdPlayer)
	require.NotEmpty(t, createdPlayer.ID)

	const selectPlayer = `SELECT id, team_id, name, position, market_value FROM players WHERE id = $1`

	// Precondition: the player belongs to the team.
	var stored player
	require.NoError(t, footballDB.GetContext(ctx, &stored, selectPlayer, createdPlayer.ID))
	require.NotNil(t, stored.TeamID)
	require.Equal(t, createdTeam.ID, *stored.TeamID)

	// Delete the team.
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL+"/teams/"+createdTeam.ID, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// The team is gone.
	var teamCount int
	require.NoError(t, footballDB.GetContext(ctx, &teamCount,
		`SELECT COUNT(*) FROM teams WHERE id = $1`, createdTeam.ID))
	assert.Equal(t, 0, teamCount, "team should be deleted")

	// The player survives as a free agent.
	require.NoError(t, footballDB.GetContext(ctx, &stored, selectPlayer, createdPlayer.ID))
	assert.Equal(t, createdPlayer.ID, stored.ID)
	assert.Nil(t, stored.TeamID, "player's team_id should be NULL after the team is deleted")
}

// TestMigrationsApplied confirms the compose migration jobs ran, which is the
// behaviour that replaced the docker-entrypoint-initdb.d mount.
func TestMigrationsApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stack test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), stackTimeout)
	defer cancel()

	stack := startStack(ctx, t)

	dbHost, dbPort := endpoint(ctx, t, stack, footballDBService, postgresPort)
	footballDB, err := db.Connect(db.Config{
		Host:     dbHost,
		Port:     dbPort,
		User:     "user",
		Password: "password",
		DBName:   "football_db",
	})
	require.NoError(t, err)
	defer footballDB.Close()

	// golang-migrate records applied versions here.
	var version int
	require.NoError(t, footballDB.GetContext(ctx, &version,
		`SELECT version FROM schema_migrations WHERE NOT dirty`))
	assert.GreaterOrEqual(t, version, 2, "index migration 000002 should be applied")

	// The indexes from 000002 must exist.
	var indexCount int
	require.NoError(t, footballDB.GetContext(ctx, &indexCount,
		`SELECT COUNT(*) FROM pg_indexes WHERE indexname IN ('idx_players_team_id', 'idx_teams_league_id')`))
	assert.Equal(t, 2, indexCount)
}
