package main

import (
	"log"
	"net/http"

	"github.com/scoutpulse/identity-svc/internal/handler"
	"github.com/scoutpulse/identity-svc/internal/repository"
	"github.com/scoutpulse/libs/db"
)

func main() {
	database, err := db.ConnectFromEnv()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	userRepo := repository.NewPostgresUserRepository(database)
	h := &handler.Handler{UserRepo: userRepo}

	http.HandleFunc("/health", h.Health)
	http.HandleFunc("/api/v1/auth/register", h.Register)
	http.HandleFunc("/api/v1/auth/login", h.Login)

	log.Println("Identity Service starting on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
