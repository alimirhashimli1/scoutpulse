package playedforboth

import (
	"math/rand/v2"
	"slices"
	"strings"
)

// The rules an online room needs on the server: drawing a pair for a level
// and knowing who played for both. They mirror played-for-both.engine.ts in
// the frontend, entity numbering included, because the browser renders the
// pair the server drew by these numbers.
//
// An entity is a club (0 .. clubs-1, in dataset order) or a national team
// (clubs .. clubs+nations-1). A pair is one integer, min*PairBase + max.

// PairBase keeps pair keys exact; wider than any entity index.
const PairBase = 100_000

// RecentWindow is how many recent pairs may not come straight back.
const RecentWindow = 60

const maxTries = 50

// Index is the dataset arranged for lookups. Read only once built, so one
// Index serves every room at once.
type Index struct {
	Data      *Dataset
	ClubCount int
	// ByEntity lists, per entity, the indexes of the players linked to it.
	ByEntity [][]int
	clubByID map[string]int
}

// NewIndex arranges a loaded dataset.
func NewIndex(d *Dataset) *Index {
	ix := &Index{
		Data:      d,
		ClubCount: len(d.Clubs),
		ByEntity:  make([][]int, len(d.Clubs)+len(d.Nations)),
		clubByID:  make(map[string]int, len(d.Clubs)),
	}
	for i, c := range d.Clubs {
		ix.clubByID[c.ID] = i
	}
	for i, p := range ix.Data.Players {
		for _, e := range ix.entities(p) {
			ix.ByEntity[e] = append(ix.ByEntity[e], i)
		}
	}
	return ix
}

func (ix *Index) entities(p Player) []int {
	out := slices.Clone(p.Clubs)
	if p.Nation >= 0 {
		out = append(out, ix.ClubCount+p.Nation)
	}
	return out
}

// PairKey joins two entities into one key, order-free.
func PairKey(a, b int) int {
	return min(a, b)*PairBase + max(a, b)
}

// SplitKey is PairKey undone.
func SplitKey(key int) (int, int) {
	return key / PairBase, key % PairBase
}

// Matches returns the players linked to both entities, as indexes.
func (ix *Index) Matches(a, b int) []int {
	if a < 0 || b < 0 || a >= len(ix.ByEntity) || b >= len(ix.ByEntity) {
		return nil
	}
	other := make(map[int]bool, len(ix.ByEntity[b]))
	for _, i := range ix.ByEntity[b] {
		other[i] = true
	}
	var out []int
	for _, i := range ix.ByEntity[a] {
		if other[i] {
			out = append(out, i)
		}
	}
	return out
}

// Pool is every pair a level can draw, each with at least one known answer.
type Pool struct {
	// Core pairs only teams the level puts in play.
	Core []int
	// Wider pairs a club in play with a smaller club; drawn WiderChance of
	// the time.
	Wider       []int
	WiderChance float64
}

// BuildPool collects a level's pairs from the players' side: every pair of
// their clubs in play, each such club with their nation when the nation is
// allowed, and, for a level with a wider chance, each such club with a
// smaller club of theirs. Never nation against nation, never a team twice.
func (ix *Index) BuildPool(l Level) Pool {
	inPlay := make([]bool, ix.ClubCount)
	wider := make([]bool, ix.ClubCount)
	leagueOn := func(codes []string, club int) bool {
		if len(codes) == 0 {
			return true
		}
		return slices.Contains(codes, ix.Data.Leagues[ix.Data.Clubs[club].League].Code)
	}
	for i := range ix.Data.Clubs {
		if len(l.Clubs) > 0 {
			inPlay[i] = slices.Contains(l.Clubs, ix.Data.Clubs[i].ID)
		} else {
			inPlay[i] = leagueOn(l.Leagues, i)
		}
		wider[i] = l.WiderChance > 0 && !inPlay[i] && leagueOn(l.WiderLeagues, i)
	}
	nationOK := func(n int) bool {
		return n >= 0 && (len(l.Nations) == 0 || slices.Contains(l.Nations, ix.Data.Nations[n].FIFA))
	}

	core, wide := map[int]bool{}, map[int]bool{}
	for _, p := range ix.Data.Players {
		if l.FamousOnly && !p.Famous {
			continue
		}
		var mine, small []int
		for _, c := range p.Clubs {
			if inPlay[c] {
				mine = append(mine, c)
			} else if wider[c] {
				small = append(small, c)
			}
		}
		for i, a := range mine {
			for _, b := range mine[i+1:] {
				if a != b {
					core[PairKey(a, b)] = true
				}
			}
			if nationOK(p.Nation) {
				core[PairKey(a, ix.ClubCount+p.Nation)] = true
			}
			for _, b := range small {
				wide[PairKey(a, b)] = true
			}
		}
	}
	return Pool{Core: sortedKeys(core), Wider: sortedKeys(wide), WiderChance: l.WiderChance}
}

// Sorted so a seeded draw is the same on every run.
func sortedKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Draw picks a pair not among the recent ones, if it can find one in
// maxTries; after that a repeat beats no pair. ok is false for an empty pool.
// Which entity is shown on the left is a coin toss.
func (p Pool) Draw(rng *rand.Rand, recent []int) (left, right int, key int, ok bool) {
	if len(p.Core) == 0 && len(p.Wider) == 0 {
		return 0, 0, 0, false
	}
	for range maxTries {
		list := p.Core
		if len(p.Wider) > 0 && (len(p.Core) == 0 || rng.Float64() < p.WiderChance) {
			list = p.Wider
		}
		key = list[rng.IntN(len(list))]
		if !slices.Contains(recent, key) {
			break
		}
	}
	left, right = SplitKey(key)
	if rng.IntN(2) == 0 {
		left, right = right, left
	}
	return left, right, key, true
}

// PushRecent appends a key, keeping the newest RecentWindow.
func PushRecent(recent []int, key int) []int {
	recent = append(recent, key)
	if len(recent) > RecentWindow {
		recent = recent[len(recent)-RecentWindow:]
	}
	return recent
}

// EntityName is a club's or nation's English name, for logs and tests.
func (ix *Index) EntityName(e int) string {
	if e < ix.ClubCount {
		return ix.Data.Clubs[e].Name
	}
	return ix.Data.Nations[e-ix.ClubCount].NameEN
}

// ClubEntity finds a club by id.
func (ix *Index) ClubEntity(id string) (int, bool) {
	e, ok := ix.clubByID[strings.ToLower(id)]
	return e, ok
}
