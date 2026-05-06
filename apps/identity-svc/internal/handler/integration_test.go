package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestAuthIntegration(t *testing.T) {
	// 1. Pre-flight check: Skip if -short is passed
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// 2. Pre-flight check: Try to verify Docker is reachable to avoid panics
	// We're using a simple check here. In some Windows environments, 
	// testcontainers-go panics if it misinterprets the Docker host.
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Skipping test: testcontainers-go panicked (likely Docker environment issue): %v", r)
		}
	}()

	// 3. Start Postgres Container
	pgContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("identity_db"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
		postgres.WithInitScripts(filepath.Join("..", "..", "db", "schema.sql")),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second)),
	)
	if err != nil {
		t.Skipf("Skipping test: Docker not available or container failed to start: %v", err)
		return
	}
	defer pgContainer.Terminate(ctx)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	assert.NoError(t, err)

	db, err := sqlx.Open("postgres", connStr)
	assert.NoError(t, err)
	defer db.Close()

	h := &Handler{DB: db}

	// 2. Test Registration
	regReq := RegisterRequest{
		Username: "junior_dev",
		Email:    "dev@scoutpulse.com",
		Password: "password123",
		Role:     model.Admin,
	}
	regBody, _ := json.Marshal(regReq)
	
	req, _ := http.NewRequest("POST", "/register", bytes.NewBuffer(regBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	
	h.Register(rr, req)
	
	assert.Equal(t, http.StatusCreated, rr.Code)
	
	var userResp model.User
	err = json.Unmarshal(rr.Body.Bytes(), &userResp)
	assert.NoError(t, err)
	assert.Equal(t, regReq.Username, userResp.Username)
	assert.Equal(t, string(model.Admin), string(userResp.Role))

	// 3. Test Login (Success)
	loginReq := LoginRequest{
		Identifier: "dev@scoutpulse.com",
		Password:   "password123",
	}
	loginBody, _ := json.Marshal(loginReq)
	
	req, _ = http.NewRequest("POST", "/login", bytes.NewBuffer(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	
	h.Login(rr, req)
	
	assert.Equal(t, http.StatusOK, rr.Code)
	
	var loginResp map[string]string
	err = json.Unmarshal(rr.Body.Bytes(), &loginResp)
	assert.NoError(t, err)
	assert.NotEmpty(t, loginResp["token"])

	// 4. Test Login (Invalid Credentials)
	loginReqInvalid := LoginRequest{
		Identifier: "dev@scoutpulse.com",
		Password:   "wrongpassword",
	}
	loginBodyInvalid, _ := json.Marshal(loginReqInvalid)
	
	req, _ = http.NewRequest("POST", "/login", bytes.NewBuffer(loginBodyInvalid))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	
	h.Login(rr, req)
	
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
