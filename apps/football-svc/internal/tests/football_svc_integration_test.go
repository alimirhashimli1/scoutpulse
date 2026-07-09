package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/handler"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestFootballServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// 1. Start Postgres Container
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpassword",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections"),
	}

	postgresC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	assert.NoError(t, err)
	defer postgresC.Terminate(ctx)

	host, _ := postgresC.Host(ctx)
	port, _ := postgresC.MappedPort(ctx, "5432")

	dbURL := fmt.Sprintf("host=%s port=%s user=testuser password=testpassword dbname=testdb sslmode=disable", host, port.Port())
	db, err := sqlx.Connect("postgres", dbURL)
	assert.NoError(t, err)
	defer db.Close()

	// 2. Apply Migrations
	migrationPath := filepath.Join("..", "..", "migrations", "000001_init_football_schema.up.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	assert.NoError(t, err)

	_, err = db.Exec(string(migrationSQL))
	assert.NoError(t, err)

	// 3. Initialize App (Service layer)
	leagueRepo := repository.NewPostgresLeagueRepository(db)
	teamRepo := repository.NewPostgresTeamRepository(db)
	coachRepo := repository.NewPostgresCoachRepository(db)

	leagueSvc := service.NewLeagueService(leagueRepo)
	teamSvc := service.NewTeamService(teamRepo)
	coachSvc := service.NewCoachService(coachRepo)

	leagueHandler := handler.NewLeagueHandler(leagueSvc)
	teamHandler := handler.NewTeamHandler(teamSvc, nil, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/leagues", leagueHandler.ListLeagues)
	mux.HandleFunc("GET /api/v1/teams", teamHandler.ListTeams)

	server := httptest.NewServer(mux)
	defer server.Close()

	// 4. Seed Data
	league := &domain.League{Name: "Premier League", Country: "England"}
	err = leagueRepo.Create(ctx, league)
	assert.NoError(t, err)

	team := &domain.Team{LeagueID: &league.ID, Name: "Arsenal"}
	err = teamRepo.Create(ctx, team)
	assert.NoError(t, err)

	// 5. Verify Endpoints
	t.Run("GET /api/v1/leagues returns JSON array", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/leagues")
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var leagues []domain.League
		err = json.NewDecoder(resp.Body).Decode(&leagues)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(leagues), 1)

		found := false
		for _, l := range leagues {
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

		var teams []domain.Team
		err = json.NewDecoder(resp.Body).Decode(&teams)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(teams), 1)

		found := false
		for _, tm := range teams {
			if tm.Name == "Arsenal" {
				found = true
				break
			}
		}
		assert.True(t, found, "Arsenal should be in the response")
	})
}
