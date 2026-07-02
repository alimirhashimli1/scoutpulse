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

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/db"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go/modules/compose"
)

func TestDockerComposeStack(t *testing.T) {
	composeFile := filepath.Join("..", "..", "docker-compose.yml")
	
	stack, err := compose.NewDockerCompose(composeFile)
	assert.NoError(t, err, "NewDockerCompose should not return an error")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start the stack
	err = stack.Up(ctx)
	
	assert.NoError(t, err, "Docker Compose stack should start without error")

	// Cleanup
	defer func() {
		err := stack.Down(ctx, compose.RemoveOrphans(true), compose.RemoveVolumes(true))
		assert.NoError(t, err, "Docker Compose stack should shut down without error")
	}()

	// Verify identity-db is running (basic check)
	services := []string{"identity-db", "football-db"}
	for _, svc := range services {
		// In a real test, we would check port availability or connection strings
		t.Logf("Service %s is part of the stack", svc)
	}
}

func TestTeamDeletionAndPlayerNulling(t *testing.T) {
	composeFile := filepath.Join("..", "..", "docker-compose.yml")

	stack, err := compose.NewDockerCompose(composeFile)
	assert.NoError(t, err, "NewDockerCompose should not return an error")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start the stack and wait for services to be healthy
	err = stack.Up(ctx, compose.Wait(true))
	assert.NoError(t, err, "Docker Compose stack should start without error")

	// Cleanup
	defer func() {
		err := stack.Down(ctx, compose.RemoveOrphans(true), compose.RemoveVolumes(true))
		assert.NoError(t, err, "Docker Compose stack should shut down without error")
	}()

	// Get football-svc container information to make HTTP requests
	footballSvcContainer, err := stack.Service("football-svc").Container(ctx)
	assert.NoError(t, err, "failed to get football-svc container")
	footballSvcHost, err := footballSvcContainer.Host(ctx)
	assert.NoError(t, err, "failed to get football-svc host")
	footballSvcPort, err := footballSvcContainer.MappedPort(ctx, "8080") // Assuming 8080 is the football-svc port
	assert.NoError(t, err, "failed to get football-svc mapped port")
	footballSvcURL := fmt.Sprintf("http://%s:%s", footballSvcHost, footballSvcPort.Port())

	// Get football-db container information to connect directly
	footballDbContainer, err := stack.Service("football-db").Container(ctx)
	assert.NoError(t, err, "failed to get football-db container")
	footballDbHost, err := footballDbContainer.Host(ctx)
	assert.NoError(t, err, "failed to get football-db host")
	footballDbPort, err := footballDbContainer.MappedPort(ctx, "5432") // Assuming 5432 is the football-db port
	assert.NoError(t, err, "failed to get football-db mapped port")

	// Set up DB config for direct connection to football-db
	dbConfig := db.Config{
		Host:     footballDbHost,
		Port:     footballDbPort.Port(),
		User:     "user",
		Password: "password",
		DBName:   "football",
	}

	// Connect to football-db directly
	footballDB, err := db.Connect(dbConfig)
	assert.NoError(t, err, "Failed to connect to football-db")
	defer footballDB.Close()

	// Generate Admin token for authenticated requests
	adminToken, err := auth.GenerateToken("test-admin", "admin", []string{})
	assert.NoError(t, err, "Failed to generate admin token")

	httpClient := &http.Client{Timeout: 10 * time.Second}

	// --- Create League ---
	leagueID := "test-league-1"
	league := domain.League{ID: leagueID, Name: "Test League", Country: "Test Country"}
	leagueBody, _ := json.Marshal(league)
	req, _ := http.NewRequestWithContext(ctx, "POST", footballSvcURL+"/leagues", bytes.NewBuffer(leagueBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := httpClient.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// --- Create Team ---
	teamID := "test-team-1"
	team := domain.Team{ID: teamID, LeagueID: &leagueID, Name: "Test Team"}
	teamBody, _ := json.Marshal(team)
	req, _ = http.NewRequestWithContext(ctx, "POST", footballSvcURL+"/teams", bytes.NewBuffer(teamBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = httpClient.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// --- Create Player ---
	playerID := "test-player-1"
	player := domain.Player{ID: playerID, TeamID: &teamID, Name: "Test Player", Position: "Forward", MarketValue: 100.0}
	playerBody, _ := json.Marshal(player)
	req, _ = http.NewRequestWithContext(ctx, "POST", footballSvcURL+"/players", bytes.NewBuffer(playerBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = httpClient.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Verify player is initially linked to the team
	var retrievedPlayer domain.Player
	err = footballDB.GetContext(ctx, &retrievedPlayer, "SELECT id, team_id, name FROM players WHERE id = $1", playerID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedPlayer.TeamID)
	assert.Equal(t, teamID, *retrievedPlayer.TeamID)

	// --- Delete Team ---
	req, _ = http.NewRequestWithContext(ctx, "DELETE", footballSvcURL+"/teams/"+teamID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err = httpClient.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode) // Assuming successful deletion returns 204

	// --- Assertions post-deletion ---

	// Verify team is deleted from the database
	var count int
	err = footballDB.GetContext(ctx, &count, "SELECT COUNT(*) FROM teams WHERE id = $1", teamID)
	assert.NoError(t, err)
	assert.Equal(t, 0, count, "Team should be deleted from the database")

	// Verify player still exists and team_id is NULL
	err = footballDB.GetContext(ctx, &retrievedPlayer, "SELECT id, team_id, name FROM players WHERE id = $1", playerID)
	assert.NoError(t, err)
	assert.Equal(t, playerID, retrievedPlayer.ID)
	assert.Nil(t, retrievedPlayer.TeamID, "Player's team_id should be NULL after team deletion")
}
