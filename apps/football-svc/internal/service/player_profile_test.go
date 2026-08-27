package service

import (
	"strings"
	"testing"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// base returns a player that passes validation, so each test changes one thing.
func base() *domain.Player {
	return &domain.Player{Name: "Erling Haaland", Position: "Centre-Forward"}
}

func TestValidatePlayerLists(t *testing.T) {
	t.Run("blank entries are cleaned rather than rejected", func(t *testing.T) {
		// A form with an empty last row is a UI artefact. Making someone go back
		// and delete it teaches them the form is fussy, not that they were wrong.
		p := base()
		p.Nationalities = domain.StringList{"Norway", "  ", ""}
		p.Strengths = domain.StringList{" Finishing ", ""}

		require.NoError(t, validatePlayer(p))
		assert.Equal(t, domain.StringList{"Norway"}, p.Nationalities)
		assert.Equal(t, domain.StringList{"Finishing"}, p.Strengths)
	})

	t.Run("a secondary position may not repeat the main one", func(t *testing.T) {
		p := base()
		p.SecondaryPositions = domain.StringList{"centre-forward"}

		err := validatePlayer(p)

		require.Error(t, err)
		assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
	})

	t.Run("caps the lists", func(t *testing.T) {
		tooMany := func(n int) domain.StringList {
			out := make(domain.StringList, n)
			for i := range out {
				out[i] = "x"
			}
			return out
		}

		p := base()
		p.Nationalities = tooMany(maxNationalities + 1)
		assert.Error(t, validatePlayer(p))

		p = base()
		p.Strengths = tooMany(maxAssessmentItems + 1)
		assert.Error(t, validatePlayer(p))
	})

	t.Run("an assessment entry is bounded but generous", func(t *testing.T) {
		p := base()
		p.Weaknesses = domain.StringList{strings.Repeat("a", maxAssessmentItemLength)}
		assert.NoError(t, validatePlayer(p), "the limit itself must be allowed")

		p = base()
		p.Weaknesses = domain.StringList{strings.Repeat("a", maxAssessmentItemLength+1)}
		assert.Error(t, validatePlayer(p))
	})

	t.Run("counts entry length in runes, not bytes", func(t *testing.T) {
		// A point written in Turkish is measured the way its author counts it.
		p := base()
		p.Strengths = domain.StringList{strings.Repeat("ş", maxAssessmentItemLength)}
		assert.NoError(t, validatePlayer(p))
	})
}

func TestValidatePlayerPercentages(t *testing.T) {
	pct := func(v float64) *float64 { return &v }

	t.Run("accepts the whole range", func(t *testing.T) {
		p := base()
		p.DuelsWonPct = pct(0)
		p.PassCompletionPct = pct(87.4)
		p.ShotsOnTargetPct = pct(100)
		assert.NoError(t, validatePlayer(p))
	})

	t.Run("rejects out-of-range values and names the field", func(t *testing.T) {
		p := base()
		p.AerialDuelsWonPct = pct(101)

		err := validatePlayer(p)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "aerial_duels_won_pct")
	})

	t.Run("nil is not recorded, and is not zero", func(t *testing.T) {
		// The distinction matters on the profile: a blank cell and "0% of duels
		// won" are very different claims about a player.
		p := base()
		require.NoError(t, validatePlayer(p))
		assert.Nil(t, p.DuelsWonPct)
	})
}

func TestValidateNoteBody(t *testing.T) {
	t.Run("trims", func(t *testing.T) {
		got, err := validateNoteBody("  reads the game well  ")
		require.NoError(t, err)
		assert.Equal(t, "reads the game well", got)
	})

	t.Run("whitespace alone is empty", func(t *testing.T) {
		_, err := validateNoteBody("   \n\t ")
		assert.Error(t, err)
	})

	t.Run("bounded, but not tightly", func(t *testing.T) {
		_, err := validateNoteBody(strings.Repeat("a", MaxNoteLength))
		assert.NoError(t, err, "the limit itself must be allowed")

		_, err = validateNoteBody(strings.Repeat("a", MaxNoteLength+1))
		assert.Error(t, err)
	})

	t.Run("counts runes, so a Turkish note is not cut short", func(t *testing.T) {
		_, err := validateNoteBody(strings.Repeat("ğ", MaxNoteLength))
		assert.NoError(t, err)
	})
}
