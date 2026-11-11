package rooms

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/scoutpulse/football-svc/internal/games/playedforboth"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clock is a hand-wound time source with its own scheduled callbacks, so a
// round can be timed out in a test without waiting for it.
type clock struct {
	mu    sync.Mutex
	now   time.Time
	tasks []*task
}

type task struct {
	at      time.Time
	f       func()
	stopped bool
}

func (t *task) Stop() bool { t.stopped = true; return true }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) after(d time.Duration, f func()) stopper {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &task{at: c.now.Add(d), f: f}
	c.tasks = append(c.tasks, t)
	return t
}

// advance moves time on, running what falls due, in order.
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	end := c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		var next *task
		for _, t := range c.tasks {
			if !t.stopped && !t.at.After(end) && (next == nil || t.at.Before(next.at)) {
				next = t
			}
		}
		if next == nil {
			c.now = end
			c.mu.Unlock()
			return
		}
		next.stopped = true
		c.now = next.at
		c.mu.Unlock()
		next.f()
	}
}

var (
	ixOnce sync.Once
	ix     *playedforboth.Index
)

func newHub(t *testing.T) (*Hub, *clock) {
	t.Helper()
	ixOnce.Do(func() {
		d, err := playedforboth.Load()
		require.NoError(t, err)
		ix = playedforboth.NewIndex(d)
	})
	c := &clock{now: time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)}
	h := NewHub(ix)
	h.now = c.Now
	h.after = c.after
	return h, c
}

// watch subscribes and returns a function giving the newest snapshot.
func watch(t *testing.T, r *Room, tok string) (func() Snapshot, func()) {
	t.Helper()
	ch, cancel, err := r.Subscribe(tok)
	require.NoError(t, err)
	var last Snapshot
	return func() Snapshot {
		for {
			select {
			case body, ok := <-ch:
				if !ok {
					return last
				}
				require.NoError(t, json.Unmarshal(body, &last))
			default:
				return last
			}
		}
	}, cancel
}

func answerFor(r *Room) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.hub.ix.Matches(r.left, r.right)
	return r.hub.ix.Data.Players[m[0]].Name
}

func twoPlayers(t *testing.T) (*Hub, *clock, *Room, Seat, Seat) {
	h, c := newHub(t)
	host, err := h.Create("Ali")
	require.NoError(t, err)
	r, err := h.Room(host.Code)
	require.NoError(t, err)
	guest, err := r.Join("Bora")
	require.NoError(t, err)
	return h, c, r, host, guest
}

func TestCreateAndJoin(t *testing.T) {
	h, _, r, host, guest := twoPlayers(t)
	assert.Len(t, host.Code, CodeLength)
	lower, err := h.Room(" " + string([]rune(host.Code)) + " ")
	require.NoError(t, err)
	assert.Same(t, r, lower)

	s := r.Snapshot()
	assert.Equal(t, PhaseLobby, s.Phase)
	assert.Equal(t, host.PlayerID, s.Host)
	require.Len(t, s.Players, 2)
	assert.Equal(t, "Bora", s.Players[1].Name)
	assert.NotEqual(t, host.Token, guest.Token)

	same, err := r.Join("bora")
	require.NoError(t, err)
	assert.Equal(t, "bora 2", r.Snapshot().Players[2].Name, "names stay apart")
	_ = same

	_, err = h.Room("NOPE1")
	assert.Equal(t, apperr.KindNotFound, apperr.KindOf(err))
}

func TestOnlyTheHostRunsTheGame(t *testing.T) {
	_, _, r, host, guest := twoPlayers(t)
	assert.Equal(t, apperr.KindForbidden, apperr.KindOf(r.Start(guest.Token)))
	assert.Equal(t, apperr.KindForbidden, apperr.KindOf(r.Configure(guest.Token, DefaultSettings)))
	assert.Equal(t, apperr.KindUnauthorized, apperr.KindOf(r.Start("forged")))
	assert.Equal(t, apperr.KindInvalid, apperr.KindOf(r.Configure(host.Token, Settings{Level: "extreme", Timer: 30, Target: 5})))
	require.NoError(t, r.Configure(host.Token, Settings{Level: playedforboth.LevelEasy, Timer: 20, Target: 3}))
	require.NoError(t, r.Start(host.Token))
	assert.Equal(t, apperr.KindConflict, apperr.KindOf(r.Configure(host.Token, DefaultSettings)))
}

func TestFirstRightAnswerWinsTheRound(t *testing.T) {
	_, c, r, host, guest := twoPlayers(t)
	latest, cancel := watch(t, r, guest.Token)
	defer cancel()
	require.NoError(t, r.Start(host.Token))
	s := latest()
	require.Equal(t, PhasePlaying, s.Phase)
	require.NotNil(t, s.Pair)
	assert.Equal(t, int64(30_000), s.Pair.RemainingMs)

	wrong, err := r.Guess(host.Token, "Nobody Atall")
	require.NoError(t, err)
	assert.False(t, wrong.Correct)

	c.advance(time.Second)
	name := answerFor(r)
	got, err := r.Guess(guest.Token, name)
	require.NoError(t, err)
	assert.True(t, got.Correct)

	c.advance(time.Second)
	late, err := r.Guess(host.Token, name)
	require.NoError(t, err)
	assert.False(t, late.Correct)
	assert.True(t, late.Late)

	s = latest()
	assert.Equal(t, PhaseReveal, s.Phase)
	require.NotNil(t, s.Win)
	assert.Equal(t, guest.PlayerID, s.Win.PlayerID)
	assert.Equal(t, name, s.Win.Answer)
	assert.Equal(t, 1, s.Players[1].Score)
	assert.Equal(t, 0, s.Players[0].Score)

	c.advance(RevealPause)
	s = latest()
	assert.Equal(t, PhasePlaying, s.Phase)
	assert.Equal(t, 2, s.Round)
	assert.Nil(t, s.Win)
}

func TestTimeRunsOut(t *testing.T) {
	_, c, r, host, _ := twoPlayers(t)
	require.NoError(t, r.Start(host.Token))
	c.advance(29 * time.Second)
	assert.Equal(t, PhasePlaying, r.Snapshot().Phase)
	assert.Equal(t, int64(1000), r.Snapshot().Pair.RemainingMs)
	c.advance(time.Second)
	s := r.Snapshot()
	assert.Equal(t, PhaseReveal, s.Phase)
	assert.Nil(t, s.Win, "nobody took it")
	c.advance(RevealPause)
	assert.Equal(t, 2, r.Snapshot().Round)
}

func TestReachingTheTargetEndsTheGame(t *testing.T) {
	_, c, r, host, guest := twoPlayers(t)
	require.NoError(t, r.Configure(host.Token, Settings{Level: playedforboth.LevelMedium, Timer: 30, Target: 3}))
	require.NoError(t, r.Start(host.Token))
	for i := range 3 {
		c.advance(time.Second)
		res, err := r.Guess(host.Token, answerFor(r))
		require.NoError(t, err)
		require.True(t, res.Correct)
		if i < 2 {
			c.advance(RevealPause)
		}
	}
	s := r.Snapshot()
	assert.Equal(t, PhaseFinished, s.Phase)
	assert.Equal(t, host.PlayerID, s.Champion)

	c.advance(time.Minute)
	assert.Equal(t, PhaseFinished, r.Snapshot().Phase, "a finished game waits for a rematch")

	require.NoError(t, r.Start(host.Token))
	s = r.Snapshot()
	assert.Equal(t, PhasePlaying, s.Phase)
	assert.Equal(t, 1, s.Round)
	assert.Equal(t, 0, s.Players[0].Score)
	assert.Empty(t, s.Champion)
	_ = guest
}

func TestHostSkips(t *testing.T) {
	_, _, r, host, guest := twoPlayers(t)
	require.NoError(t, r.Start(host.Token))
	assert.Equal(t, apperr.KindForbidden, apperr.KindOf(r.Next(guest.Token)))
	require.NoError(t, r.Next(host.Token))
	assert.Equal(t, PhaseReveal, r.Snapshot().Phase)
	require.NoError(t, r.Next(host.Token))
	s := r.Snapshot()
	assert.Equal(t, PhasePlaying, s.Phase)
	assert.Equal(t, 2, s.Round)
}

func TestGuessingTooFastIsRefused(t *testing.T) {
	_, c, r, host, _ := twoPlayers(t)
	require.NoError(t, r.Start(host.Token))
	_, err := r.Guess(host.Token, "first try")
	require.NoError(t, err)
	_, err = r.Guess(host.Token, "second try")
	assert.Equal(t, apperr.KindRateLimited, apperr.KindOf(err))
	c.advance(guessGap)
	_, err = r.Guess(host.Token, "third try")
	assert.NoError(t, err)
}

func TestHostHandsOverWhenGone(t *testing.T) {
	h, c, r, host, guest := twoPlayers(t)
	_, cancelHost := watch(t, r, host.Token)
	_, cancelGuest := watch(t, r, guest.Token)
	defer cancelGuest()
	cancelHost()

	c.advance(hostGrace / 2)
	h.sweep()
	assert.Equal(t, host.PlayerID, r.Snapshot().Host, "a reload is not leaving")

	c.advance(hostGrace)
	h.sweep()
	assert.Equal(t, guest.PlayerID, r.Snapshot().Host)
	assert.False(t, r.Snapshot().Players[0].Online)
}

func TestLeaving(t *testing.T) {
	h, _, r, host, guest := twoPlayers(t)
	require.NoError(t, r.Leave(host.Token))
	s := r.Snapshot()
	require.Len(t, s.Players, 1)
	assert.Equal(t, guest.PlayerID, s.Host)

	require.NoError(t, r.Leave(guest.Token))
	_, err := h.Room(host.Code)
	assert.Equal(t, apperr.KindNotFound, apperr.KindOf(err), "the last one out closes the room")
}

func TestIdleRoomsAreSwept(t *testing.T) {
	h, c, _, host, _ := twoPlayers(t)
	c.advance(idleRoom + time.Second)
	h.sweep()
	_, err := h.Room(host.Code)
	assert.Equal(t, apperr.KindNotFound, apperr.KindOf(err))
}

func TestCloseEndsStreams(t *testing.T) {
	h, _, r, host, _ := twoPlayers(t)
	ch, cancel, err := r.Subscribe(host.Token)
	require.NoError(t, err)
	defer cancel()
	h.Close()
	for range ch {
	}
	_, err = h.Create("late")
	assert.Error(t, err)
}

func TestFullRoom(t *testing.T) {
	_, _, r, _, _ := twoPlayers(t)
	for range MaxPlayers - 2 {
		_, err := r.Join("x")
		require.NoError(t, err)
	}
	_, err := r.Join("one too many")
	assert.Equal(t, apperr.KindConflict, apperr.KindOf(err))
}

func TestCleanName(t *testing.T) {
	assert.Equal(t, "Player 3", cleanName("  ‎ ", 3))
	assert.Equal(t, "Ali Veli", cleanName(" Ali \n  Veli ", 1))
	assert.Len(t, []rune(cleanName("abcdefghijklmnopqrstuvwxyz", 1)), MaxNameLength)
}

func TestPollingKeepsAPlayerOnline(t *testing.T) {
	h, c, r, host, guest := twoPlayers(t)
	_, err := r.Poll("forged")
	assert.Equal(t, apperr.KindUnauthorized, apperr.KindOf(err))

	s, err := r.Poll(guest.Token)
	require.NoError(t, err)
	assert.True(t, s.Players[1].Online)
	assert.False(t, s.Players[0].Online, "the host neither streams nor polls")

	c.advance(pollGrace + time.Second)
	assert.False(t, r.Snapshot().Players[1].Online)

	// A host who only polls keeps the room.
	for range 6 {
		_, err = r.Poll(host.Token)
		require.NoError(t, err)
		c.advance(4 * time.Second)
		h.sweep()
	}
	assert.Equal(t, host.PlayerID, r.Snapshot().Host)
}
