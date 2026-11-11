package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scoutpulse/football-svc/internal/games/playedforboth"
	"github.com/scoutpulse/football-svc/internal/games/rooms"
	"github.com/scoutpulse/libs/platform/observability"
	"github.com/scoutpulse/libs/platform/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const roomsBase = "/api/v1/games/played-for-both/rooms"

// roomServer is the real middleware chain, with timeouts far shorter than
// production's, so a stream that did not lift them would die mid-test.
func roomServer(t *testing.T) (*httptest.Server, *GameRoomHandler) {
	t.Helper()
	d, err := playedforboth.Load()
	require.NoError(t, err)
	hub := rooms.NewHub(playedforboth.NewIndex(d))
	rh := NewGameRoomHandler(hub)
	rh.heartbeat = 100 * time.Millisecond
	mux := http.NewServeMux()
	RegisterRoutes(mux, Handlers{GameRoom: rh})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	chain := server.Chain(mux,
		server.RequestID, server.Recover(logger), server.Logging(logger), server.CORS([]string{"*"}),
		observability.Tracing("test"), observability.Metrics("test"), server.Recover(logger),
	)
	srv := httptest.NewUnstartedServer(chain)
	srv.Config.ReadTimeout = 300 * time.Millisecond
	srv.Config.WriteTimeout = 300 * time.Millisecond
	srv.Start()
	// The hub first: closing it ends the streams, which the server would
	// otherwise wait for.
	t.Cleanup(func() { hub.Close(); srv.Close() })
	return srv, rh
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	res, err := http.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func seat(t *testing.T, res *http.Response) rooms.Seat {
	t.Helper()
	require.Equal(t, http.StatusCreated, res.StatusCode)
	var s rooms.Seat
	require.NoError(t, json.NewDecoder(res.Body).Decode(&s))
	return s
}

// events reads snapshots off a stream as they come.
func events(t *testing.T, url string) <-chan rooms.Snapshot {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", "text/event-stream")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
	out := make(chan rooms.Snapshot, 32)
	go func() {
		defer close(out)
		defer func() { _ = res.Body.Close() }()
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok && data != "{}" {
				var s rooms.Snapshot
				if json.Unmarshal([]byte(data), &s) == nil {
					out <- s
				}
			}
		}
	}()
	return out
}

func next(t *testing.T, ch <-chan rooms.Snapshot, ok func(rooms.Snapshot) bool) rooms.Snapshot {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case s, open := <-ch:
			require.True(t, open, "stream ended early")
			if ok(s) {
				return s
			}
		case <-deadline:
			t.Fatal("no matching snapshot")
		}
	}
}

func TestRoomOverHTTP(t *testing.T) {
	srv, _ := roomServer(t)
	host := seat(t, postJSON(t, srv.URL+roomsBase, map[string]string{"name": "Ali"}))
	room := srv.URL + roomsBase + "/" + host.Code
	guest := seat(t, postJSON(t, room+"/players", map[string]string{"name": "Bora"}))

	stream := events(t, room+"/events?token="+guest.Token)
	first := next(t, stream, func(rooms.Snapshot) bool { return true })
	assert.Equal(t, rooms.PhaseLobby, first.Phase)

	res := postJSON(t, room+"/start", map[string]string{"token": guest.Token})
	assert.Equal(t, http.StatusForbidden, res.StatusCode)

	req, _ := http.NewRequest(http.MethodPut, room+"/settings",
		strings.NewReader(`{"token":"`+host.Token+`","level":"easy","timer":20,"target":3}`))
	req.Header.Set("Content-Type", "application/json")
	put, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = put.Body.Close()
	assert.Equal(t, http.StatusNoContent, put.StatusCode)

	// Outlive the server's read and write timeouts before the game starts.
	time.Sleep(700 * time.Millisecond)

	assert.Equal(t, http.StatusNoContent, postJSON(t, room+"/start", map[string]string{"token": host.Token}).StatusCode)
	playing := next(t, stream, func(s rooms.Snapshot) bool { return s.Phase == rooms.PhasePlaying })
	assert.Equal(t, "easy", playing.Settings.Level)
	require.NotNil(t, playing.Pair)

	res = postJSON(t, room+"/guesses", map[string]string{"token": guest.Token, "text": "Definitely Wrong"})
	var verdict rooms.GuessResult
	require.NoError(t, json.NewDecoder(res.Body).Decode(&verdict))
	assert.False(t, verdict.Correct)

	// Leaving by beacon: text/plain.
	beacon, err := http.Post(room+"/leave", "text/plain;charset=UTF-8", strings.NewReader(`{"token":"`+host.Token+`"}`))
	require.NoError(t, err)
	_ = beacon.Body.Close()
	assert.Equal(t, http.StatusNoContent, beacon.StatusCode)
	after := next(t, stream, func(s rooms.Snapshot) bool { return len(s.Players) == 1 })
	assert.Equal(t, guest.PlayerID, after.Host)
}

func TestRoomStreamNeedsASeat(t *testing.T) {
	srv, _ := roomServer(t)
	host := seat(t, postJSON(t, srv.URL+roomsBase, map[string]string{"name": "Ali"}))
	res, err := http.Get(srv.URL + roomsBase + "/" + host.Code + "/events?token=nope")
	require.NoError(t, err)
	_ = res.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)

	res, err = http.Get(srv.URL + roomsBase + "/ZZZZZ/events?token=" + host.Token)
	require.NoError(t, err)
	_ = res.Body.Close()
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}
