package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/scoutpulse/football-svc/db"
	"github.com/scoutpulse/libs/auth"
)

func main() {
	database, err := db.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Football Service is healthy"))
	})

	mux.HandleFunc("/players", func(w http.ResponseWriter, r *http.Request) {
		// This is PUBLIC - anyone can see the list of players
		response := map[string]interface{}{
			"message": "Public football data",
			"players": []string{"Lionel Messi", "Cristiano Ronaldo", "Kylian Mbappé"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Admin-only routes - for future tasks like ADDING or DELETING players
	mux.Handle("/admin/players", auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, _ := auth.GetClaims(r.Context())
		
		if claims.Role != "admin" {
			http.Error(w, "Forbidden: Admin access only", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Welcome, Admin! You have permission to modify player data.",
		})
	})))

	port := ":8081"
	fmt.Printf("Football Service starting on port %s\n", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
