package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/scoutpulse/football-svc/internal/games/rooms"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
	"github.com/scoutpulse/libs/platform/server"
)

// GameRoomHandler is the online mode of "Who Played For Both?".
//
// Public, like the rest of the games: a room is joined with its code and each
// player proves who they are with the token they got on joining. Changes are
// pushed back as server-sent events rather than over a WebSocket: the
// traffic is one-way (state out, actions in as plain POSTs), so a stream
// needs no new dependency, passes through the gateway untouched, and the
// browser reconnects it by itself.
type GameRoomHandler struct {
	hub *rooms.Hub
	// seats limits rooms opened and joined per address; a room is memory.
	// The burst covers a full room joining at once from one network: a
	// party or a class on the same Wi-Fi shares one public address.
	seats *server.RateLimit
	// streamLife ends a stream after a while, so that no connection is
	// open forever; the browser reconnects at once and lands on the newest
	// state. Replaced in tests.
	streamLife time.Duration
	heartbeat  time.Duration
}

// NewGameRoomHandler serves the rooms of hub.
func NewGameRoomHandler(hub *rooms.Hub) *GameRoomHandler {
	return &GameRoomHandler{
		hub:        hub,
		seats:      server.NewRateLimit(0.5, rooms.MaxPlayers+10),
		streamLife: 10 * time.Minute,
		heartbeat:  10 * time.Second,
	}
}

func (h *GameRoomHandler) register(mux *http.ServeMux) {
	const base = "/api/v1/games/played-for-both/rooms"
	mux.Handle("POST "+base, h.seats.Middleware(http.HandlerFunc(h.Create)))
	mux.Handle("POST "+base+"/{code}/players", h.seats.Middleware(http.HandlerFunc(h.Join)))
	mux.HandleFunc("GET "+base+"/{code}/events", h.Events)
	mux.HandleFunc("GET "+base+"/{code}", h.Poll)
	mux.HandleFunc("PUT "+base+"/{code}/settings", h.Configure)
	mux.HandleFunc("POST "+base+"/{code}/start", h.action((*rooms.Room).Start))
	mux.HandleFunc("POST "+base+"/{code}/next", h.action((*rooms.Room).Next))
	mux.HandleFunc("POST "+base+"/{code}/stop", h.action((*rooms.Room).Stop))
	mux.HandleFunc("POST "+base+"/{code}/leave", h.action((*rooms.Room).Leave))
	mux.HandleFunc("POST "+base+"/{code}/guesses", h.Guess)
}

type nameBody struct {
	Name string `json:"name"`
}

type tokenBody struct {
	Token string `json:"token"`
}

// Create opens a room; the caller is its host.
func (h *GameRoomHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body nameBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	seat, err := h.hub.Create(body.Name)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, seat)
}

// Join seats the caller in a room.
func (h *GameRoomHandler) Join(w http.ResponseWriter, r *http.Request) {
	var body nameBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	room, err := h.hub.Room(r.PathValue("code"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	seat, err := room.Join(body.Name)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, seat)
}

// Configure changes the room's settings.
func (h *GameRoomHandler) Configure(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
		rooms.Settings
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	room, err := h.hub.Room(r.PathValue("code"))
	if err == nil {
		err = room.Configure(body.Token, body.Settings)
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Guess checks a typed name.
func (h *GameRoomHandler) Guess(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
		Text  string `json:"text"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	room, err := h.hub.Room(r.PathValue("code"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := room.Guess(body.Token, body.Text)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// action serves a call that needs only the player's token.
//
// The body is read whatever its content type, because a player closing the
// tab leaves with navigator.sendBeacon, which can only send text/plain.
func (h *GameRoomHandler) action(do func(*rooms.Room, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body tokenBody
		raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<10))
		if err != nil || json.Unmarshal(raw, &body) != nil {
			httpx.WriteError(w, r, apperr.Invalid("expected {\"token\": \"...\"}"))
			return
		}
		room, err := h.hub.Room(r.PathValue("code"))
		if err == nil {
			err = do(room, body.Token)
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Poll returns the room once: the fallback for a browser whose event
// stream is held up on the way.
func (h *GameRoomHandler) Poll(w http.ResponseWriter, r *http.Request) {
	room, err := h.hub.Room(r.PathValue("code"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	snap, err := room.Poll(r.URL.Query().Get("token"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, snap)
}

// Events streams the room's snapshots to one player, as server-sent events.
//
// The token is in the query because EventSource cannot send headers. It only
// ever names a seat in one short-lived room.
func (h *GameRoomHandler) Events(w http.ResponseWriter, r *http.Request) {
	room, err := h.hub.Room(r.PathValue("code"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	updates, cancel, err := room.Subscribe(r.URL.Query().Get("token"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer cancel()

	// The server's read and write timeouts are sized for ordinary requests
	// and would cut the stream off after seconds. This one ends by itself
	// at streamLife instead.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-store")
	hd.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// How soon to reconnect after a drop, in milliseconds.
	if _, err := io.WriteString(w, "retry: 1500\n\n"); err != nil {
		return
	}
	_ = rc.Flush()

	beat := time.NewTicker(h.heartbeat)
	defer beat.Stop()
	life := time.NewTimer(h.streamLife)
	defer life.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-life.C:
			return
		case <-beat.C:
			// Keeps proxies from closing an idle stream, and tells the page
			// the stream is alive; a comment line would not reach its script.
			if _, err := io.WriteString(w, "event: ping\ndata: {}\n\n"); err != nil {
				return
			}
		case body, ok := <-updates:
			if !ok {
				// The room closed, or this player left it.
				_, _ = io.WriteString(w, "event: closed\ndata: {}\n\n")
				_ = rc.Flush()
				return
			}
			if _, err := io.WriteString(w, "data: "+string(body)+"\n\n"); err != nil {
				return
			}
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
