package main

import (
	"log"
	"net/http"
	"github.com/scoutpulse/identity-svc/internal/db"
	"github.com/scoutpulse/identity-svc/internal/handler"
	"github.com/scoutpulse/identity-svc/internal/repository"
)

func main() {
	database, err := db.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	userRepo := repository.NewPostgresUserRepository(database)
	h := &handler.Handler{UserRepo: userRepo}

	http.HandleFunc("/register", h.Register)
	http.HandleFunc("/login", h.Login)

	log.Println("Identity Service starting on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
