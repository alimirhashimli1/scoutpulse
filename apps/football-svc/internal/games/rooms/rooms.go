// Package rooms runs online games of "Who Played For Both?".
//
// A room lives in this process's memory and nowhere else: a game lasts
// minutes, nothing about it is worth keeping, and a database round trip per
// keystroke-race would be the slowest part of the game. The cost is that a
// deploy ends the games in progress, and that the service must run as one
// instance for two players to meet in the same room. Both are fine for a
// party game.
//
// Every change is pushed to the room's subscribers as a full snapshot, not a
// diff, so a browser that missed one -- or reconnected -- is right again on
// the next.
package rooms

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	mrand "math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/scoutpulse/football-svc/internal/games/playedforboth"
	"github.com/scoutpulse/libs/platform/apperr"
)

const (
	// MaxPlayers in one room.
	MaxPlayers = 8
	// MaxRooms open at once, so a script cannot fill the memory.
	MaxRooms = 1000
	// MaxNameLength in runes.
	MaxNameLength = 20

	// RevealPause is how long the answer stays up before the next pair.
	RevealPause = 5 * time.Second
	// guessGap is the least time between one player's guesses, against a
	// script that tries every name it knows.
	guessGap = 250 * time.Millisecond
	// idleRoom is how long a room with nobody watching is kept, for a
	// player who reloads or loses signal.
	idleRoom = 10 * time.Minute
	// maxRoomAge closes even a busy room eventually.
	maxRoomAge = 6 * time.Hour
	// hostGrace is how long a host may be gone before someone else is host.
	hostGrace = 20 * time.Second
	// pollGrace is how long a player who polls instead of streaming counts
	// as connected after their last poll.
	pollGrace = 5 * time.Second
)

// Phase is where a room is in its game.
type Phase string

const (
	PhaseLobby    Phase = "lobby"
	PhasePlaying  Phase = "playing"
	PhaseReveal   Phase = "reveal"
	PhaseFinished Phase = "finished"
)

// Settings are the host's choices.
type Settings struct {
	Level string `json:"level"`
	// Timer is the seconds a pair stays up unanswered.
	Timer int `json:"timer"`
	// Target is the points that win the game; 0 plays on until the host stops.
	Target int `json:"target"`
}

var (
	timers  = []int{20, 30, 60}
	targets = []int{0, 3, 5, 10}
)

// DefaultSettings for a new room.
var DefaultSettings = Settings{Level: playedforboth.LevelMedium, Timer: 30, Target: 5}

func (s Settings) valid() bool {
	_, ok := playedforboth.LevelByCode(s.Level)
	return ok && contains(timers, s.Timer) && contains(targets, s.Target)
}

func contains(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Seat is what a player gets on joining: who they are, and the secret that
// proves it on every later call.
type Seat struct {
	Code     string `json:"code"`
	PlayerID string `json:"playerId"`
	Token    string `json:"token"`
}

type player struct {
	id, name, token string
	score           int
	conns           int
	away            time.Time // when the last connection closed
	polled          time.Time // the last poll, for a player without a stream
	lastGuess       time.Time
}

// online: streaming, or polling lately.
func (p *player) online(now time.Time) bool {
	return p.conns > 0 || now.Sub(p.polled) < pollGrace
}

// gone is when the player was last known to be there.
func (p *player) gone() time.Time {
	if p.polled.After(p.away) {
		return p.polled
	}
	return p.away
}

// Hub holds every room.
type Hub struct {
	ix    *playedforboth.Index
	pools map[string]playedforboth.Pool
	now   func() time.Time
	// after schedules a callback; replaced in tests.
	after func(time.Duration, func()) stopper

	mu     sync.Mutex
	rooms  map[string]*Room
	closed bool
	stop   chan struct{}
}

type stopper interface{ Stop() bool }

// NewHub builds the pools of every level up front: a few milliseconds once,
// rather than on a player's first click.
func NewHub(ix *playedforboth.Index) *Hub {
	h := &Hub{
		ix:    ix,
		pools: make(map[string]playedforboth.Pool, len(playedforboth.Levels)),
		now:   time.Now,
		after: func(d time.Duration, f func()) stopper { return time.AfterFunc(d, f) },
		rooms: make(map[string]*Room),
		stop:  make(chan struct{}),
	}
	for _, l := range playedforboth.Levels {
		h.pools[l.Code] = ix.BuildPool(l)
	}
	return h
}

// Run sweeps idle rooms until Close. Meant for its own goroutine.
func (h *Hub) Run() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-t.C:
			h.sweep()
		}
	}
}

// Close ends every room and every stream, for shutdown: an open stream
// would otherwise hold the server's drain until it times out.
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	close(h.stop)
	rooms := h.rooms
	h.rooms = map[string]*Room{}
	h.mu.Unlock()
	for _, r := range rooms {
		r.shut()
	}
}

func (h *Hub) sweep() {
	now := h.now()
	h.mu.Lock()
	var dead []*Room
	for code, r := range h.rooms {
		if r.expired(now) {
			delete(h.rooms, code)
			dead = append(dead, r)
		}
	}
	live := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		live = append(live, r)
	}
	h.mu.Unlock()
	for _, r := range dead {
		r.shut()
	}
	for _, r := range live {
		r.handOverHost(now)
	}
}

// Create opens a room with its creator as host.
func (h *Hub) Create(name string) (Seat, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return Seat{}, apperr.RateLimited("the game server is restarting, try again in a moment")
	}
	if len(h.rooms) >= MaxRooms {
		return Seat{}, apperr.RateLimited("too many games are running, try again soon")
	}
	code := ""
	for code == "" || h.rooms[code] != nil {
		code = roomCode()
	}
	r := &Room{
		hub:      h,
		code:     code,
		settings: DefaultSettings,
		phase:    PhaseLobby,
		created:  h.now(),
		active:   h.now(),
		subs:     map[chan []byte]string{},
		rng:      mrand.New(mrand.NewPCG(seed(), seed())),
	}
	p := r.addLocked(name)
	r.host = p.id
	h.rooms[code] = r
	return Seat{Code: code, PlayerID: p.id, Token: p.token}, nil
}

// Room finds an open room by its code, in any case.
func (h *Hub) Room(code string) (*Room, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.rooms[strings.ToUpper(strings.TrimSpace(code))]
	if r == nil {
		return nil, apperr.NotFound("no game with that code; check it, or start a new one")
	}
	return r, nil
}

func (h *Hub) remove(r *Room) {
	h.mu.Lock()
	if h.rooms[r.code] == r {
		delete(h.rooms, r.code)
	}
	h.mu.Unlock()
}

// Room is one game.
type Room struct {
	hub  *Hub
	code string

	mu       sync.Mutex
	players  []*player
	nextID   int
	host     string
	settings Settings
	phase    Phase
	round    int
	left     int
	right    int
	hasPair  bool
	endsAt   time.Time
	win      *RoundWin
	champion string
	recent   []int
	timer    stopper
	// gen invalidates a scheduled callback that lost a race with a newer
	// change: each one runs only if gen is still what it was when scheduled.
	gen     int
	version int
	created time.Time
	active  time.Time
	subs    map[chan []byte]string // stream → player id
	done    bool
	rng     *mrand.Rand
}

// RoundWin says who took a round, and with which name.
type RoundWin struct {
	PlayerID string `json:"player"`
	Answer   string `json:"answer"`
}

// Snapshot is the whole room as every player sees it.
type Snapshot struct {
	Code     string       `json:"code"`
	Version  int          `json:"version"`
	Phase    Phase        `json:"phase"`
	Round    int          `json:"round"`
	Host     string       `json:"host"`
	Players  []PlayerView `json:"players"`
	Settings Settings     `json:"settings"`
	Pair     *PairView    `json:"pair"`
	Win      *RoundWin    `json:"win"`
	Champion string       `json:"champion"`
}

// PlayerView is one line of the scoreboard.
type PlayerView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Online bool   `json:"online"`
}

// PairView is the pair on screen, by entity, plus the clock. Time is sent as
// what is left rather than a deadline, so the players' clocks need not agree
// with the server's.
type PairView struct {
	Left        int   `json:"left"`
	Right       int   `json:"right"`
	RemainingMs int64 `json:"remainingMs"`
	DurationMs  int64 `json:"durationMs"`
}

// Join seats a new player.
func (r *Room) Join(name string) (Seat, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return Seat{}, apperr.NotFound("that game has ended")
	}
	if len(r.players) >= MaxPlayers {
		return Seat{}, apperr.Conflict("that game is full")
	}
	p := r.addLocked(name)
	r.changedLocked()
	return Seat{Code: r.code, PlayerID: p.id, Token: p.token}, nil
}

func (r *Room) addLocked(name string) *player {
	r.nextID++
	p := &player{
		id:    "p" + strconv.Itoa(r.nextID),
		token: token(),
		away:  r.hub.now(),
	}
	p.name = r.uniqueNameLocked(cleanName(name, r.nextID))
	r.players = append(r.players, p)
	return p
}

func (r *Room) uniqueNameLocked(name string) string {
	taken := func(n string) bool {
		for _, p := range r.players {
			if strings.EqualFold(p.name, n) {
				return true
			}
		}
		return false
	}
	if !taken(name) {
		return name
	}
	for i := 2; ; i++ {
		n := name + " " + strconv.Itoa(i)
		if !taken(n) {
			return n
		}
	}
}

func (r *Room) byToken(tok string) (*player, error) {
	for _, p := range r.players {
		if tok != "" && p.token == tok {
			return p, nil
		}
	}
	return nil, apperr.Unauthorized("you are not in this game")
}

func (r *Room) hostLocked(tok string) (*player, error) {
	p, err := r.byToken(tok)
	if err != nil {
		return nil, err
	}
	if p.id != r.host {
		return nil, apperr.Forbidden("only the host can do that")
	}
	return p, nil
}

// Subscribe opens a stream of snapshots for a seated player. The first one
// is sent straight away. cancel must be called when the stream ends.
func (r *Room) Subscribe(tok string) (<-chan []byte, func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, err := r.byToken(tok)
	if err != nil {
		return nil, nil, err
	}
	if r.done {
		return nil, nil, apperr.NotFound("that game has ended")
	}
	ch := make(chan []byte, 1)
	r.subs[ch] = p.id
	p.conns++
	r.active = r.hub.now()
	r.changedLocked()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if _, ok := r.subs[ch]; !ok {
				return // already closed by shut
			}
			delete(r.subs, ch)
			close(ch)
			for _, q := range r.players {
				if q.id == p.id {
					q.conns--
					if q.conns == 0 {
						q.away = r.hub.now()
					}
				}
			}
			r.active = r.hub.now()
			r.changedLocked()
		})
	}
	return ch, cancel, nil
}

// Poll returns the room now, for a player whose stream does not get
// through (a proxy that buffers it, say). Polling keeps the player online.
func (r *Room) Poll(tok string) (Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, err := r.byToken(tok)
	if err != nil {
		return Snapshot{}, err
	}
	if r.done {
		return Snapshot{}, apperr.NotFound("that game has ended")
	}
	now := r.hub.now()
	wasOnline := p.online(now)
	p.polled = now
	r.active = now
	if !wasOnline {
		r.changedLocked()
	}
	return r.snapshotLocked(), nil
}

// Configure changes the settings; the host only, between games.
func (r *Room) Configure(tok string, s Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.hostLocked(tok); err != nil {
		return err
	}
	if !s.valid() {
		return apperr.Invalid("unknown level, timer or target")
	}
	if r.phase != PhaseLobby && r.phase != PhaseFinished {
		return apperr.Conflict("settings change between games")
	}
	r.settings = s
	r.changedLocked()
	return nil
}

// Start begins a game, or a rematch, from zero. The host only.
func (r *Room) Start(tok string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.hostLocked(tok); err != nil {
		return err
	}
	if r.phase == PhasePlaying || r.phase == PhaseReveal {
		return apperr.Conflict("the game is already on")
	}
	for _, p := range r.players {
		p.score = 0
	}
	r.round = 0
	r.champion = ""
	r.nextRoundLocked()
	return nil
}

// Next skips an unanswered pair, or cuts the reveal short. The host only.
func (r *Room) Next(tok string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.hostLocked(tok); err != nil {
		return err
	}
	switch r.phase {
	case PhasePlaying:
		r.revealLocked(nil)
	case PhaseReveal:
		r.nextRoundLocked()
	default:
		return apperr.Conflict("no pair to move on from")
	}
	return nil
}

// Stop ends the game early and goes back to the lobby. The host only.
func (r *Room) Stop(tok string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.hostLocked(tok); err != nil {
		return err
	}
	r.cancelTimerLocked()
	r.phase = PhaseLobby
	r.hasPair = false
	r.win = nil
	r.changedLocked()
	return nil
}

// GuessResult says whether a guess took the round.
type GuessResult struct {
	Correct bool `json:"correct"`
	// Late is true when the guess was right but someone else got there first.
	Late bool `json:"late,omitempty"`
}

// Guess checks a typed name against the pair on screen. The first right one
// wins the round.
func (r *Room) Guess(tok, text string) (GuessResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, err := r.byToken(tok)
	if err != nil {
		return GuessResult{}, err
	}
	if len([]rune(text)) > playedforboth.MaxGuessLength {
		return GuessResult{}, apperr.Invalid("that is longer than any name")
	}
	now := r.hub.now()
	if now.Sub(p.lastGuess) < guessGap {
		return GuessResult{}, apperr.RateLimited("slow down")
	}
	p.lastGuess = now
	r.active = now

	if r.phase != PhasePlaying {
		if r.phase == PhaseReveal && r.hasPair && r.answerLocked(text) != "" {
			return GuessResult{Late: true}, nil
		}
		return GuessResult{}, nil
	}
	name := r.answerLocked(text)
	if name == "" {
		return GuessResult{}, nil
	}
	p.score++
	r.revealLocked(&RoundWin{PlayerID: p.id, Answer: name})
	return GuessResult{Correct: true}, nil
}

func (r *Room) answerLocked(text string) string {
	ix := r.hub.ix
	i, ok := ix.FindAnswer(text, ix.Matches(r.left, r.right))
	if !ok {
		return ""
	}
	return ix.Data.Players[i].Name
}

// Leave takes a player out of the room. The last one out closes it.
func (r *Room) Leave(tok string) error {
	r.mu.Lock()
	p, err := r.byToken(tok)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	kept := r.players[:0]
	for _, q := range r.players {
		if q != p {
			kept = append(kept, q)
		}
	}
	r.players = kept
	if len(r.players) == 0 {
		r.mu.Unlock()
		r.hub.remove(r)
		r.shut()
		return nil
	}
	if r.host == p.id {
		r.host = r.bestHostLocked()
	}
	// Their streams end with them.
	for ch, id := range r.subs {
		if id == p.id {
			delete(r.subs, ch)
			close(ch)
		}
	}
	r.changedLocked()
	r.mu.Unlock()
	return nil
}

// bestHostLocked is the first player still connected, else the first.
func (r *Room) bestHostLocked() string {
	for _, q := range r.players {
		if q.online(r.hub.now()) {
			return q.id
		}
	}
	return r.players[0].id
}

func (r *Room) handOverHost(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.players {
		if p.id == r.host && !p.online(now) && now.Sub(p.gone()) > hostGrace {
			if next := r.bestHostLocked(); next != r.host {
				r.host = next
				r.changedLocked()
			}
			return
		}
	}
}

func (r *Room) expired(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now.Sub(r.created) > maxRoomAge {
		return true
	}
	return len(r.subs) == 0 && now.Sub(r.active) > idleRoom
}

// --- rounds -------------------------------------------------------------

func (r *Room) nextRoundLocked() {
	r.cancelTimerLocked()
	pool := r.hub.pools[r.settings.Level]
	left, right, key, ok := pool.Draw(r.rng, r.recent)
	if !ok {
		// Cannot happen with the shipped levels; a lobby is the honest state.
		r.phase = PhaseLobby
		r.changedLocked()
		return
	}
	r.recent = playedforboth.PushRecent(r.recent, key)
	r.round++
	r.left, r.right, r.hasPair = left, right, true
	r.win = nil
	r.phase = PhasePlaying
	r.endsAt = r.hub.now().Add(time.Duration(r.settings.Timer) * time.Second)
	r.scheduleLocked(time.Duration(r.settings.Timer)*time.Second, func() { r.revealLocked(nil) })
	r.changedLocked()
}

// revealLocked ends the round: won by someone, or by nobody when time ran
// out or the host skipped.
func (r *Room) revealLocked(win *RoundWin) {
	r.cancelTimerLocked()
	r.win = win
	if win != nil && r.settings.Target > 0 {
		for _, p := range r.players {
			if p.id == win.PlayerID && p.score >= r.settings.Target {
				r.phase = PhaseFinished
				r.champion = p.id
				r.changedLocked()
				return
			}
		}
	}
	r.phase = PhaseReveal
	r.scheduleLocked(RevealPause, r.nextRoundLocked)
	r.changedLocked()
}

func (r *Room) scheduleLocked(d time.Duration, f func()) {
	r.gen++
	gen := r.gen
	r.timer = r.hub.after(d, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.gen == gen && !r.done {
			f()
		}
	})
}

func (r *Room) cancelTimerLocked() {
	r.gen++
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
}

// --- pushing ------------------------------------------------------------

func (r *Room) snapshotLocked() Snapshot {
	s := Snapshot{
		Code:     r.code,
		Version:  r.version,
		Phase:    r.phase,
		Round:    r.round,
		Host:     r.host,
		Settings: r.settings,
		Win:      r.win,
		Champion: r.champion,
		Players:  make([]PlayerView, 0, len(r.players)),
	}
	now := r.hub.now()
	for _, p := range r.players {
		s.Players = append(s.Players, PlayerView{ID: p.id, Name: p.name, Score: p.score, Online: p.online(now)})
	}
	if r.hasPair && r.phase != PhaseLobby {
		left := int64(0)
		if r.phase == PhasePlaying {
			left = max(0, r.endsAt.Sub(now).Milliseconds())
		}
		s.Pair = &PairView{
			Left: r.left, Right: r.right,
			RemainingMs: left,
			DurationMs:  int64(r.settings.Timer) * 1000,
		}
	}
	return s
}

// Snapshot is the room now, for a caller outside a stream.
func (r *Room) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

// changedLocked bumps the version and pushes the new state to every stream.
// A stream that has not read the last one has it replaced: only the newest
// state matters.
func (r *Room) changedLocked() {
	r.version++
	body, err := json.Marshal(r.snapshotLocked())
	if err != nil {
		return
	}
	for ch := range r.subs {
		select {
		case ch <- body:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- body:
			default:
			}
		}
	}
}

func (r *Room) shut() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return
	}
	r.done = true
	r.cancelTimerLocked()
	for ch := range r.subs {
		delete(r.subs, ch)
		close(ch)
	}
}

// --- small helpers ------------------------------------------------------

// Letters and digits that cannot be misread for each other.
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// CodeLength of a room code.
const CodeLength = 5

func roomCode() string {
	b := make([]byte, CodeLength)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b)
}

func token() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func seed() uint64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	var s uint64
	for _, x := range b {
		s = s<<8 | uint64(x)
	}
	return s
}

func cleanName(name string, n int) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
	}
	out := []rune(strings.Join(strings.Fields(b.String()), " "))
	if len(out) > MaxNameLength {
		out = out[:MaxNameLength]
	}
	if len(out) == 0 {
		return "Player " + strconv.Itoa(n)
	}
	return string(out)
}
