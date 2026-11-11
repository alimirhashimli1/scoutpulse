package playedforboth

import (
	"encoding/json"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func index(t *testing.T) *Index {
	t.Helper()
	d, err := Load()
	require.NoError(t, err)
	return NewIndex(d)
}

func TestLevelsNameRealTeams(t *testing.T) {
	ix := index(t)
	leagues := map[string]bool{}
	for _, l := range ix.Data.Leagues {
		leagues[l.Code] = true
	}
	nations := map[string]bool{}
	for _, n := range ix.Data.Nations {
		nations[n.FIFA] = true
	}
	require.Equal(t, []string{LevelEasy, LevelMedium, LevelHard}, []string{Levels[0].Code, Levels[1].Code, Levels[2].Code})
	for _, l := range Levels {
		for _, c := range l.Clubs {
			_, ok := ix.ClubEntity(c)
			assert.True(t, ok, "%s: club %q", l.Code, c)
		}
		for _, code := range append(slices.Clone(l.Leagues), l.WiderLeagues...) {
			assert.True(t, leagues[code], "%s: league %q", l.Code, code)
		}
		for _, n := range l.Nations {
			assert.True(t, nations[n], "%s: nation %q", l.Code, n)
		}
	}
}

func TestServedCarriesTheLevels(t *testing.T) {
	body, err := Served()
	require.NoError(t, err)
	var got struct {
		Players []json.RawMessage `json:"players"`
		Levels  []Level           `json:"levels"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	assert.NotEmpty(t, got.Players)
	assert.Equal(t, Levels, got.Levels)
}

// Each level is harder than the last: more pairs, from more teams.
func TestLevelsGrowHarder(t *testing.T) {
	ix := index(t)
	easy, medium, hard := ix.BuildPool(Levels[0]), ix.BuildPool(Levels[1]), ix.BuildPool(Levels[2])
	assert.Less(t, len(easy.Core), len(medium.Core))
	assert.Less(t, len(medium.Core), len(hard.Core))
	assert.NotEmpty(t, easy.Wider)
	assert.Empty(t, medium.Wider)
	t.Logf("pairs: easy %d (+%d wider), medium %d, hard %d", len(easy.Core), len(easy.Wider), len(medium.Core), len(hard.Core))
}

func TestEasyDrawsBigClubsMostly(t *testing.T) {
	ix := index(t)
	l := Levels[0]
	pool := ix.BuildPool(l)
	rng := rand.New(rand.NewPCG(1, 2))
	big := func(e int) bool {
		if e >= ix.ClubCount {
			return slices.Contains(l.Nations, ix.Data.Nations[e-ix.ClubCount].FIFA)
		}
		return slices.Contains(l.Clubs, ix.Data.Clubs[e].ID)
	}
	const draws = 2000
	lower := 0
	var recent []int
	for range draws {
		a, b, key, ok := pool.Draw(rng, recent)
		require.True(t, ok)
		recent = PushRecent(recent, key)
		require.NotEmpty(t, ix.Matches(a, b), "%s v %s", ix.EntityName(a), ix.EntityName(b))
		require.True(t, big(a) || big(b), "%s v %s", ix.EntityName(a), ix.EntityName(b))
		if !big(a) || !big(b) {
			lower++
		}
	}
	share := float64(lower) / draws
	assert.InDelta(t, l.WiderChance, share, 0.05, "share of pairs with a smaller club")
}

func TestMediumStaysInTheMainLeagues(t *testing.T) {
	ix := index(t)
	l := Levels[1]
	pool := ix.BuildPool(l)
	for _, key := range pool.Core {
		for _, e := range []int{key / PairBase, key % PairBase} {
			if e < ix.ClubCount {
				code := ix.Data.Leagues[ix.Data.Clubs[e].League].Code
				require.Contains(t, l.Leagues, code, ix.EntityName(e))
			}
		}
	}
}

func TestDrawAvoidsRecentPairs(t *testing.T) {
	ix := index(t)
	pool := ix.BuildPool(Levels[1])
	rng := rand.New(rand.NewPCG(3, 4))
	var recent []int
	seen := map[int]bool{}
	for range RecentWindow {
		_, _, key, _ := pool.Draw(rng, recent)
		require.False(t, seen[key], "repeat inside the window")
		seen[key] = true
		recent = PushRecent(recent, key)
	}
}

func TestNameMatches(t *testing.T) {
	cases := []struct {
		guess, name string
		want        bool
	}{
		{"Cristiano Ronaldo", "Cristiano Ronaldo", true},
		{"ronaldo", "Cristiano Ronaldo", true},
		{"cristiano", "Cristiano Ronaldo", true},
		{"Ronaldinho", "Ronaldinho", true},
		{"van persie", "Robin van Persie", true},
		{"persie", "Robin van Persie", true},
		{"De Bruyne", "Kevin De Bruyne", true},
		{"ozil", "Mesut Özil", true},
		{"Kante", "N'Golo Kanté", true},
		{"ngolo", "N'Golo Kanté", true},
		{"Wan-Bissaka", "Aaron Wan-Bissaka", true},
		{"bissaka", "Aaron Wan-Bissaka", true},
		{"Ødegaard", "Martin Ødegaard", true},
		{"odegaard", "Martin Ødegaard", true},
		{"Ibrahimovic", "Zlatan Ibrahimović", true},
		{"ibrahimovich", "Zlatan Ibrahimović", true}, // one slip
		{"Lewandowsky", "Robert Lewandowski", true},
		{"Ronaldp", "Cristiano Ronaldo", true},
		{"Rinaldi", "Cristiano Ronaldo", false},
		{"van", "Robin van Persie", false},
		{"da", "David da Silva", false},
		{"", "Cristiano Ronaldo", false},
		{"Messi", "Lionel Andrés Messi", true},
		{"Andres", "Lionel Andrés Messi", false},
		{"Kun", "Kun Agüero", true},
		{"Pique", "Gerard Piqué", true},
		{"Pepe", "Pepe", true},
		{"Pepa", "Pepe", false}, // too short to forgive
		{"anything", "", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, NameMatches(c.guess, c.name), "%q for %q", c.guess, c.name)
	}
}

func TestFindAnswerPrefersTheExactName(t *testing.T) {
	ix := index(t)
	rma, _ := ix.ClubEntity("rma")
	mun, _ := ix.ClubEntity("mun")
	players := ix.Matches(rma, mun)
	require.NotEmpty(t, players)
	i, ok := ix.FindAnswer("cristiano ronaldo", players)
	require.True(t, ok)
	assert.Equal(t, "Cristiano Ronaldo", ix.Data.Players[i].Name)
	_, ok = ix.FindAnswer("Lionel Messi", players)
	assert.False(t, ok)
}
