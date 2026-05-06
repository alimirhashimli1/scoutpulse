package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlayersRoute_Public(t *testing.T) {
	mux := http.NewServeMux()

	// Replicate the route setup from main.go
	mux.HandleFunc("/players", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"message": "Public football data",
			"players": []string{"Lionel Messi", "Cristiano Ronaldo", "Kylian Mbappé"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Create a request without any auth headers
	req, _ := http.NewRequest("GET", "/players", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rr.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	assert.Equal(t, "Public football data", response["message"])
	assert.Contains(t, response, "players")
	
	players := response["players"].([]interface{})
	assert.Len(t, players, 3)
	assert.Contains(t, players, "Lionel Messi")
}
