package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/handler"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestFootballServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// testcontainers-go panics rather than returning an error when it cannot
	// resolve a Docker host, so recover and skip instead of failing the suite
	// on machines and CI runners without a daemon.
	//
	// containerStarted bounds that leniency. This block used to swallow *every*
	// panic and report it as a missing daemon, which turned a genuine failure
	// -- a connection reset while Postgres was still restarting -- into the
	// message "likely no Docker daemon" on a runner that had Docker working
	// fine. Once the container is up, a panic is a real failure and is
	// re-raised.
	containerStarted := false
	defer func() {
		if rec := recover(); rec != nil {
			if containerStarted {
				panic(rec)
			}
			t.Skipf("Skipping test: testcontainers-go panicked before the container started "+
				"(likely no Docker daemon): %v", rec)
		}
	}()

	// 1. Start Postgres Container
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpassword",
			"POSTGRES_DB":       "testdb",
		},
		// The official image logs "ready to accept connections" TWICE, and
		// waiting for the first one is a race that fails as "connection reset
		// by peer".
		//
		// The first is the temporary server initdb runs to execute the
		// bootstrap scripts; it listens on a Unix socket only and is then shut
		// down. The second is the real server, listening on TCP. Connecting
		// after the first means connecting while it is being restarted.
		//
		// WithOccurrence(2) waits for the real one; the port check then
		// confirms it is actually accepting TCP, since the log line is written
		// just before the socket is ready.
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithDeadline(2 * time.Minute),
	}

	postgresC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		// Without a Docker daemon there is nothing to test against. Skip
		// rather than fail, so the suite stays usable on machines and CI
		// runners that do not provide one.
		t.Skipf("Skipping integration test, Docker is unavailable: %v", err)
	}
	containerStarted = true
	defer func() {
		_ = postgresC.Terminate(ctx)
	}()

	host, err := postgresC.Host(ctx)
	require.NoError(t, err, "resolving container host")
	port, err := postgresC.MappedPort(ctx, "5432")
	require.NoError(t, err, "resolving mapped port")

	// require, not assert, from here down: these are preconditions, and
	// continuing past a failed one meant running the whole test against a nil
	// *sqlx.DB. That surfaced as a nil-pointer panic several lines later,
	// which the recover above then reported as a missing Docker daemon --
	// three layers of misdirection over one connection error.
	dbURL := fmt.Sprintf("host=%s port=%s user=testuser password=testpassword dbname=testdb sslmode=disable", host, port.Port())
	db, err := sqlx.Connect("postgres", dbURL)
	require.NoError(t, err, "connecting to the test database")
	defer func() { _ = db.Close() }()

	// 2. Apply every migration in order, so the schema under test is the one
	// the service actually runs against.
	migrations, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	require.NoError(t, err)
	sort.Strings(migrations)
	require.NotEmpty(t, migrations, "expected migration files")

	for _, path := range migrations {
		migrationSQL, err := os.ReadFile(path)
		require.NoError(t, err)
		_, err = db.Exec(string(migrationSQL))
		require.NoError(t, err, "applying %s", filepath.Base(path))
	}

	// 3. Initialize App (Service layer)
	leagueRepo := repository.NewPostgresLeagueRepository(db)
	teamRepo := repository.NewPostgresTeamRepository(db)
	coachRepo := repository.NewPostgresCoachRepository(db)
	playerRepo := repository.NewPostgresPlayerRepository(db)
	teamEditorRepo := repository.NewPostgresTeamEditorRepository(db)

	authz := service.NewAuthorizer(teamEditorRepo)
	publisher := events.NopPublisher{}

	leagueSvc := service.NewLeagueService(leagueRepo, authz)
	teamSvc := service.NewTeamService(teamRepo, authz, publisher)
	coachSvc := service.NewCoachService(coachRepo, authz)
	playerSvc := service.NewPlayerService(playerRepo, authz, publisher)

	leagueHandler := handler.NewLeagueHandler(leagueSvc)
	teamHandler := handler.NewTeamHandler(teamSvc, playerSvc, coachSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/leagues", leagueHandler.ListLeagues)
	mux.HandleFunc("GET /api/v1/teams", teamHandler.ListTeams)

	server := httptest.NewServer(mux)
	defer server.Close()

	// 4. Seed Data
	//
	// Seeding goes through the repositories rather than the services, so it
	// skips the service layer's defaulting. competition_type must therefore be
	// set explicitly: the column has a DEFAULT, but an explicit empty string
	// overrides it and trips leagues_competition_type_valid.
	//
	// Callers of the API never hit this -- validateLeague fills the default in
	// before the row is written. It is only reachable from a direct repository
	// call, which is what this arrange step does.
	league := &domain.League{
		Name:            "Premier League",
		Country:         "England",
		CompetitionType: domain.CompetitionLeague,
	}
	err = leagueRepo.Create(ctx, league)
	require.NoError(t, err, "seeding the league")

	team := &domain.Team{LeagueID: &league.ID, Name: "Arsenal"}
	err = teamRepo.Create(ctx, team)
	require.NoError(t, err, "seeding the team")

	// 5. Verify Endpoints
	t.Run("GET /api/v1/leagues returns JSON array", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/leagues")
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var leagues domain.ListResult[domain.League]
		err = json.NewDecoder(resp.Body).Decode(&leagues)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(leagues.Items), 1)

		found := false
		for _, l := range leagues.Items {
			if l.Name == "Premier League" {
				found = true
				break
			}
		}
		assert.True(t, found, "Premier League should be in the response")
	})

	t.Run("GET /api/v1/teams returns JSON array", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/teams?league_id=" + league.ID)
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var teams domain.ListResult[domain.Team]
		err = json.NewDecoder(resp.Body).Decode(&teams)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(teams.Items), 1)

		found := false
		for _, tm := range teams.Items {
			if tm.Name == "Arsenal" {
				found = true
				break
			}
		}
		assert.True(t, found, "Arsenal should be in the response")
	})
}
