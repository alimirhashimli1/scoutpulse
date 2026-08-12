package service

import (
	"context"
	"testing"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLeagueRepo records what the service handed the repository.
//
// The point of these tests is what reaches the database, not what the database
// does with it, so a recorder is more direct than a mock with expectations.
type captureLeagueRepo struct {
	created *domain.League
	updated *domain.League
}

func (r *captureLeagueRepo) GetByID(context.Context, string) (*domain.League, error) {
	return &domain.League{ID: "league-1", Name: "Existing", Country: "Testland"}, nil
}
func (r *captureLeagueRepo) List(context.Context, domain.Page) ([]domain.League, error) {
	return nil, nil
}
func (r *captureLeagueRepo) Create(_ context.Context, l *domain.League) error {
	r.created = l
	return nil
}
func (r *captureLeagueRepo) Update(_ context.Context, l *domain.League) error {
	r.updated = l
	return nil
}
func (r *captureLeagueRepo) Delete(context.Context, string) error { return nil }

// TestCreateLeague_DefaultsCompetitionType is the guarantee that keeps the
// documented minimal payload working.
//
// leagues.competition_type has a NOT NULL DEFAULT, but the repository writes
// the column explicitly -- and an explicit empty string overrides a DEFAULT
// rather than falling back to it, which trips the CHECK constraint. The
// service layer is what stops that reaching the database, so a caller can post
// just a name and a country.
func TestCreateLeague_DefaultsCompetitionType(t *testing.T) {
	repo := &captureLeagueRepo{}
	svc := NewLeagueService(repo, newTestAuthorizer(t, newStubEditors()))

	err := svc.CreateLeague(ctxAs("user-admin", RoleAdmin), &domain.League{
		Name:    "Premier League",
		Country: "England",
	})

	require.NoError(t, err)
	require.NotNil(t, repo.created, "the league should have reached the repository")
	assert.Equal(t, domain.CompetitionLeague, repo.created.CompetitionType,
		"an unset competition_type must be defaulted before the insert, never written as an empty string")
}

func TestUpdateLeague_DefaultsCompetitionType(t *testing.T) {
	repo := &captureLeagueRepo{}
	svc := NewLeagueService(repo, newTestAuthorizer(t, newStubEditors()))

	err := svc.UpdateLeague(ctxAs("user-admin", RoleAdmin), &domain.League{
		ID: "league-1", Name: "Premier League", Country: "England",
	})

	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	assert.Equal(t, domain.CompetitionLeague, repo.updated.CompetitionType)
}

func TestCreateLeague_RejectsUnknownCompetitionType(t *testing.T) {
	repo := &captureLeagueRepo{}
	svc := NewLeagueService(repo, newTestAuthorizer(t, newStubEditors()))

	err := svc.CreateLeague(ctxAs("user-admin", RoleAdmin), &domain.League{
		Name: "Odd Cup", Country: "Testland", CompetitionType: "friendly_kickabout",
	})

	require.Error(t, err)
	assert.Nil(t, repo.created, "an invalid competition type must not reach the database")
}

func TestCreateLeague_AcceptsEveryValidCompetitionType(t *testing.T) {
	for _, ct := range []domain.CompetitionType{
		domain.CompetitionLeague,
		domain.CompetitionDomesticCup,
		domain.CompetitionInternational,
		domain.CompetitionSuperCup,
	} {
		t.Run(string(ct), func(t *testing.T) {
			repo := &captureLeagueRepo{}
			svc := NewLeagueService(repo, newTestAuthorizer(t, newStubEditors()))

			err := svc.CreateLeague(ctxAs("user-admin", RoleAdmin), &domain.League{
				Name: "Comp " + string(ct), Country: "Testland", CompetitionType: ct,
			})

			require.NoError(t, err)
			assert.Equal(t, ct, repo.created.CompetitionType)
		})
	}
}

func TestCreateLeague_RequiresNameAndCountry(t *testing.T) {
	tests := []struct {
		name   string
		league domain.League
	}{
		{"no name", domain.League{Country: "England"}},
		{"no country", domain.League{Name: "Premier League"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &captureLeagueRepo{}
			svc := NewLeagueService(repo, newTestAuthorizer(t, newStubEditors()))

			err := svc.CreateLeague(ctxAs("user-admin", RoleAdmin), &tt.league)

			require.Error(t, err)
			assert.Nil(t, repo.created)
		})
	}
}
