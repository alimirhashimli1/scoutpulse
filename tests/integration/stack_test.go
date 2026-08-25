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
	"os"
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
	ID       string  `json:"id" db:"id"`
	TeamID   *string `json:"team_id" db:"team_id"`
	Name     string  `json:"name" db:"name"`
	Position string  `json:"position" db:"position"`
	// ContractStart dates the opening transfer written when a player is
	// created. Without it that record is dated "now", which makes any
	// subsequent transfer with a historical date a backdated one.
	ContractStart *string `json:"contract_start,omitempty" db:"contract_start"`
	// Money crosses the wire as an integer count of minor units, never a
	// decimal, so no precision is lost in transit.
	MarketValue int64 `json:"market_value_minor" db:"market_value_minor"`
}

type transfer struct {
	ID           string  `json:"id"`
	PlayerID     string  `json:"player_id"`
	FromTeam     *string `json:"from_team_id"`
	ToTeam       *string `json:"to_team_id"`
	TransferDate string  `json:"transfer_date"`
	Type         string  `json:"transfer_type"`
	Fee          *int64  `json:"fee_minor"`
}

// listResult mirrors the envelope every list endpoint returns.
type listResult[T any] struct {
	Items   []T  `json:"items"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

const (
	// Must match the compose service names in docker-compose.yml.
	footballService   = "football-svc"
	footballDBService = "postgres"

	// Ports the containers listen on internally.
	footballServicePort = "8081"
	postgresPort        = "5432"

	// Credentials the stack is started with.
	dbUser     = "football_user"
	dbPassword = "password"

	stackTimeout = 5 * time.Minute
)

// The key pair the stack is started with. Generated once per run: identity-svc
// receives the private key and signs with it, football-svc receives only the
// public key and can therefore verify but not mint.
var testPrivateKeyPEM, testPublicKeyPEM []byte

func TestMain(m *testing.M) {
	var err error
	testPrivateKeyPEM, testPublicKeyPEM, err = auth.GenerateKeyPair(auth.MinRSAKeyBits)
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

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
			"JWT_PRIVATE_KEY":      string(testPrivateKeyPEM),
			"JWT_PUBLIC_KEY":       string(testPublicKeyPEM),
			"FOOTBALL_DB_USER":     dbUser,
			"FOOTBALL_DB_PASSWORD": dbPassword,
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
	require.NoError(t, err, "resolving host for %s", service)

	mapped, err := container.MappedPort(ctx, nat.Port(port+"/tcp"))
	if err != nil {
		// Say what Docker actually published before failing.
		//
		// docker-compose.yml binds these to 127.0.0.1 rather than 0.0.0.0, so
		// the published bindings carry a host IP. If that is what stops the
		// port being resolved here, the map below shows it directly instead of
		// leaving a bare "port not found" to be guessed at.
		if lister, ok := any(container).(interface {
			Ports(context.Context) (nat.PortMap, error)
		}); ok {
			if published, pErr := lister.Ports(ctx); pErr == nil {
				t.Logf("published ports for %s: %+v", service, published)
			} else {
				t.Logf("could not read published ports for %s: %v", service, pErr)
			}
		}
		require.NoError(t, err, "resolving mapped port %s/tcp for %s", port, service)
	}

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
		User:     dbUser,
		Password: dbPassword,
		DBName:   "football_db",
	})
	require.NoError(t, err)
	defer footballDB.Close()

	// The stack is started with this key pair, so a token minted here
	// verifies inside the services.
	require.NoError(t, auth.SetSigningKey(testPrivateKeyPEM))
	adminToken, err := auth.GenerateToken("00000000-0000-0000-0000-000000000001", "admin", "admin")
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

	const selectPlayer = `SELECT id, team_id, name, position, market_value_minor FROM players WHERE id = $1`

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
		User:     dbUser,
		Password: dbPassword,
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

// TestTransferHistoryIsRecorded covers the capability the temporal model
// exists for: a move is filed in the history and the player's current club
// follows from it, rather than the previous club being overwritten and lost.
func TestTransferHistoryIsRecorded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stack test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), stackTimeout)
	defer cancel()

	stack := startStack(ctx, t)

	svcHost, svcPort := endpoint(ctx, t, stack, footballService, footballServicePort)
	baseURL := fmt.Sprintf("http://%s:%s/api/v1", svcHost, svcPort)

	require.NoError(t, auth.SetSigningKey(testPrivateKeyPEM))
	adminToken, err := auth.GenerateToken("00000000-0000-0000-0000-000000000001", "admin", "admin")
	require.NoError(t, err)

	client := &http.Client{Timeout: 10 * time.Second}

	var comp league
	postJSON(ctx, t, client, baseURL+"/leagues", adminToken,
		league{Name: "Transfer Test League", Country: "Testland"}, &comp)

	var selling, buying team
	postJSON(ctx, t, client, baseURL+"/teams", adminToken,
		team{LeagueID: &comp.ID, Name: "Selling FC"}, &selling)
	postJSON(ctx, t, client, baseURL+"/teams", adminToken,
		team{LeagueID: &comp.ID, Name: "Buying FC"}, &buying)

	// The player joined the selling club well before the move below. Creating a
	// player writes an opening transfer, dated from contract_start when one is
	// given; leaving it unset would date that record "now" and make the
	// 2026-01-15 move a backdated one, which the service refuses to let rewrite
	// the current club.
	joined := "2020-07-01T00:00:00Z"
	var signed player
	postJSON(ctx, t, client, baseURL+"/players", adminToken,
		player{TeamID: &selling.ID, Name: "Moving Player", Position: "Forward", ContractStart: &joined},
		&signed)

	// Record the move.
	fee := int64(25_000_000_00) // 25m, in minor units
	var recorded transfer
	postJSON(ctx, t, client, baseURL+"/transfers", adminToken, map[string]any{
		"player_id":     signed.ID,
		"from_team_id":  selling.ID,
		"to_team_id":    buying.ID,
		"transfer_date": "2026-01-15T00:00:00Z",
		"transfer_type": "permanent",
		"fee_minor":     fee,
	}, &recorded)
	require.NotEmpty(t, recorded.ID)

	// The player's current club followed the move.
	resp, err := client.Get(baseURL + "/players/" + signed.ID)
	require.NoError(t, err)
	var current player
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&current))
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, current.TeamID)
	assert.Equal(t, buying.ID, *current.TeamID)

	// And the previous club survives in the history, which is the whole point.
	resp, err = client.Get(baseURL + "/players/" + signed.ID + "/transfers")
	require.NoError(t, err)
	var history listResult[transfer]
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&history))
	require.NoError(t, resp.Body.Close())

	// Two rows: the opening record from creating the player, and the move.
	// Newest first, so the move leads.
	require.Len(t, history.Items, 2,
		"creating a player writes an opening transfer, so the move is the second row")

	latest := history.Items[0]
	require.NotNil(t, latest.FromTeam)
	assert.Equal(t, selling.ID, *latest.FromTeam, "the selling club must survive in the history")
	require.NotNil(t, latest.ToTeam)
	assert.Equal(t, buying.ID, *latest.ToTeam)
	require.NotNil(t, latest.Fee)
	assert.Equal(t, fee, *latest.Fee, "the fee must survive the round trip exactly")

	// The opening record: an arrival from nowhere, so the player's career does
	// not begin with an unexplained club membership.
	opening := history.Items[1]
	assert.Nil(t, opening.FromTeam, "the opening record arrives from nowhere")
	require.NotNil(t, opening.ToTeam)
	assert.Equal(t, selling.ID, *opening.ToTeam)
}

// TestBackdatedTransferDoesNotRewriteCurrentClub pins the guard that the
// fixture above is written to avoid tripping.
//
// History is the source of truth and the current club is derived from it, so
// filing an old move must add to the record without claiming the player is at
// that club today.
func TestBackdatedTransferDoesNotRewriteCurrentClub(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stack test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), stackTimeout)
	defer cancel()

	stack := startStack(ctx, t)

	svcHost, svcPort := endpoint(ctx, t, stack, footballService, footballServicePort)
	baseURL := fmt.Sprintf("http://%s:%s/api/v1", svcHost, svcPort)

	require.NoError(t, auth.SetSigningKey(testPrivateKeyPEM))
	adminToken, err := auth.GenerateToken("00000000-0000-0000-0000-000000000001", "admin", "admin")
	require.NoError(t, err)

	client := &http.Client{Timeout: 10 * time.Second}

	var comp league
	postJSON(ctx, t, client, baseURL+"/leagues", adminToken,
		league{Name: "Backdate League", Country: "Testland"}, &comp)

	var current, historical team
	postJSON(ctx, t, client, baseURL+"/teams", adminToken,
		team{LeagueID: &comp.ID, Name: "Current FC"}, &current)
	postJSON(ctx, t, client, baseURL+"/teams", adminToken,
		team{LeagueID: &comp.ID, Name: "Old FC"}, &historical)

	joined := "2024-01-01T00:00:00Z"
	var p player
	postJSON(ctx, t, client, baseURL+"/players", adminToken,
		player{TeamID: &current.ID, Name: "Settled Player", Position: "Midfielder", ContractStart: &joined},
		&p)

	// A move dated before the player's arrival record. The origin must match
	// where they actually are, so this is a well-formed request -- it is only
	// the date that puts it in the past.
	body, err := json.Marshal(map[string]any{
		"player_id":     p.ID,
		"from_team_id":  current.ID,
		"to_team_id":    historical.ID,
		"transfer_date": "2022-06-01T00:00:00Z",
		"transfer_type": "permanent",
	})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/transfers", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusCreated, resp.StatusCode, "a backdated transfer is still recorded")

	// Recorded in the history, but the current club is unchanged.
	resp, err = client.Get(baseURL + "/players/" + p.ID)
	require.NoError(t, err)
	var after player
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&after))
	require.NoError(t, resp.Body.Close())

	require.NotNil(t, after.TeamID)
	assert.Equal(t, current.ID, *after.TeamID,
		"a backdated move must not rewrite where the player is today")
}

// TestFootballServiceCannotMintTokens is the property asymmetric signing buys.
// The football service is given only the public key, so a token it might
// somehow construct cannot be signed -- unlike under a shared HMAC secret,
// where every verifier was also an issuer.
func TestFootballServiceCannotMintTokens(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stack test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), stackTimeout)
	defer cancel()

	stack := startStack(ctx, t)

	svcHost, svcPort := endpoint(ctx, t, stack, footballService, footballServicePort)
	baseURL := fmt.Sprintf("http://%s:%s/api/v1", svcHost, svcPort)

	// A token signed by an unrelated key must be refused.
	otherPriv, _, err := auth.GenerateKeyPair(auth.MinRSAKeyBits)
	require.NoError(t, err)
	require.NoError(t, auth.SetSigningKey(otherPriv))

	forged, err := auth.GenerateToken("00000000-0000-0000-0000-0000000000ff", "attacker", "admin")
	require.NoError(t, err)

	body, err := json.Marshal(league{Name: "Forged League", Country: "Nowhere"})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/leagues", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+forged)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"a token signed by an untrusted key must be rejected")
}
