package main

import (
	"log"
	"net/http"

	"github.com/scoutpulse/identity-svc/internal/handler"
	"github.com/scoutpulse/identity-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/db"
	"github.com/scoutpulse/libs/platform/server"
)

func main() {
	// Fail fast: without a signing key this service cannot issue usable tokens.
	if err := auth.LoadSecretFromEnv(); err != nil {
		log.Fatalf("Failed to load JWT signing key: %v", err)
	}

	database, err := db.ConnectFromEnv()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	userRepo := repository.NewPostgresUserRepository(database)
	h := &handler.Handler{UserRepo: userRepo}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("POST /api/v1/auth/register", h.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)

	if err := server.Run("identity-svc", ":8080", mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
