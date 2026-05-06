package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Football Service is healthy"))
	})

	mux.HandleFunc("/players", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"message": "Public football data",
			"players": []string{"Lionel Messi", "Cristiano Ronaldo", "Kylian Mbappé"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	port := ":8081"
	fmt.Printf("Football Service starting on port %s\n", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
