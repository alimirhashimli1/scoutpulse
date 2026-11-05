package playedforboth

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func load(t *testing.T) *Dataset {
	t.Helper()
	d, err := Load()
	require.NoError(t, err)
	return d
}

func TestDatasetCounts(t *testing.T) {
	d := load(t)
	assert.Len(t, d.Leagues, 20)
	assert.Len(t, d.Clubs, 359)
	assert.Len(t, d.Nations, 60)
	assert.InDelta(t, 11200, len(d.Players), 300, "about 11,200 players")
}

// The same spot checks the extraction script runs, so a regenerated file that
// drops a well-known career fails here too.
func TestDatasetKnownCareers(t *testing.T) {
	d := load(t)

	club := func(name string) int {
		i := slices.IndexFunc(d.Clubs, func(c Club) bool { return c.Name == name })
		require.GreaterOrEqual(t, i, 0, "club %s", name)
		return i
	}
	nation := slices.IndexFunc(d.Nations, func(n Nation) bool { return n.NameEN == "Togo" })
	require.GreaterOrEqual(t, nation, 0)

	both := func(keep func(Player) bool) []string {
		var out []string
		for _, p := range d.Players {
			if keep(p) {
				out = append(out, p.Name)
			}
		}
		return out
	}
	clubs := func(a, b int) []string {
		return both(func(p Player) bool { return slices.Contains(p.Clubs, a) && slices.Contains(p.Clubs, b) })
	}

	spurs := club("Tottenham Hotspur")
	assert.Equal(t, []string{"Emmanuel Adebayor"},
		both(func(p Player) bool { return p.Nation == nation && slices.Contains(p.Clubs, spurs) }))

	rmPsg := clubs(club("Real Madrid"), club("Paris Saint-Germain"))
	assert.Subset(t, rmPsg, []string{"Kylian Mbappé", "Sergio Ramos", "David Beckham"})

	galInter := clubs(club("Galatasaray"), club("Inter Milan"))
	assert.Subset(t, galInter, []string{"Wesley Sneijder", "Hakan Şükür"})
}

func TestFlagCodesIncludeSubdivisions(t *testing.T) {
	d := load(t)
	var flags []string
	for _, n := range d.Nations {
		flags = append(flags, n.Flag)
	}
	// flag-icons names, including the home nations.
	assert.Contains(t, flags, "gb-eng")
}
