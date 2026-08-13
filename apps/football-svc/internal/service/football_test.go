package service

import (
	"context"
	"testing"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- mocks -------------------------------------------------------------

type MockTeamRepository struct{ mock.Mock }

func (m *MockTeamRepository) GetByID(ctx context.Context, id string) (*domain.Team, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Team), args.Error(1)
}

func (m *MockTeamRepository) List(ctx context.Context, leagueID *string, page domain.Page) ([]domain.Team, error) {
	args := m.Called(ctx, leagueID, page)
	return args.Get(0).([]domain.Team), args.Error(1)
}

func (m *MockTeamRepository) Create(ctx context.Context, team *domain.Team) error {
	return m.Called(ctx, team).Error(0)
}

func (m *MockTeamRepository) Update(ctx context.Context, team *domain.Team) error {
	return m.Called(ctx, team).Error(0)
}

func (m *MockTeamRepository) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

type MockPlayerRepository struct{ mock.Mock }

func (m *MockPlayerRepository) GetByID(ctx context.Context, id string) (*domain.Player, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Player), args.Error(1)
}

func (m *MockPlayerRepository) List(ctx context.Context, filter repository.PlayerFilter, page domain.Page) ([]domain.Player, error) {
	args := m.Called(ctx, filter, page)
	return args.Get(0).([]domain.Player), args.Error(1)
}

func (m *MockPlayerRepository) Create(ctx context.Context, player *domain.Player) error {
	return m.Called(ctx, player).Error(0)
}

func (m *MockPlayerRepository) Update(ctx context.Context, player *domain.Player) error {
	return m.Called(ctx, player).Error(0)
}

func (m *MockPlayerRepository) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

type MockCoachRepository struct{ mock.Mock }

func (m *MockCoachRepository) GetByID(ctx context.Context, id string) (*domain.Coach, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Coach), args.Error(1)
}

func (m *MockCoachRepository) GetByTeam(ctx context.Context, teamID string) (*domain.Coach, error) {
	args := m.Called(ctx, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Coach), args.Error(1)
}

func (m *MockCoachRepository) Create(ctx context.Context, coach *domain.Coach) error {
	return m.Called(ctx, coach).Error(0)
}

func (m *MockCoachRepository) Update(ctx context.Context, coach *domain.Coach) error {
	return m.Called(ctx, coach).Error(0)
}

func (m *MockCoachRepository) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

// --- team ---------------------------------------------------------------

func TestTeamService_UpdateTeam_RBAC(t *testing.T) {
	const (
		editorID = "user-editor"
		teamID   = "team-1"
	)

	tests := []struct {
		name        string
		userID      string
		role        string
		grantedTeam string
		wantErr     error
	}{
		{"admin may edit any club", "user-admin", RoleAdmin, "", nil},
		{"editor with a grant may edit", editorID, RoleEditor, teamID, nil},
		{"editor without a grant may not", editorID, RoleEditor, "team-2", ErrForbidden},
		{"plain user may not", "user-plain", "user", "", ErrForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editors := newStubEditors()
			if tt.grantedTeam != "" {
				editors.grant(tt.userID, tt.grantedTeam)
			}

			repo := new(MockTeamRepository)
			svc := NewTeamService(repo, newTestAuthorizer(t, editors), &recordingPublisher{})

			// UpdateTeam reads the club first, to compare the submitted
			// league_id against the stored one.
			repo.On("GetByID", mock.Anything, teamID).
				Return(&domain.Team{ID: teamID, Name: "Old Name"}, nil).Maybe()

			team := &domain.Team{ID: teamID, Name: "New Name"}
			if tt.wantErr == nil {
				repo.On("Update", mock.Anything, team).Return(nil).Once()
			}

			err := svc.UpdateTeam(ctxAs(tt.userID, tt.role), team)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
			} else {
				assert.NoError(t, err)
				repo.AssertExpectations(t)
			}
		})
	}
}

func TestTeamService_UnauthenticatedIsRejected(t *testing.T) {
	repo := new(MockTeamRepository)
	svc := NewTeamService(repo, newTestAuthorizer(t, newStubEditors()), &recordingPublisher{})

	// No claims on the context at all.
	err := svc.UpdateTeam(context.Background(), &domain.Team{ID: "team-1", Name: "X"})

	assert.ErrorIs(t, err, ErrUnauthorized)
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestTeamService_CreateTeam_IsAdminOnly(t *testing.T) {
	editors := newStubEditors().grant("user-editor", "team-1")
	repo := new(MockTeamRepository)
	publisher := &recordingPublisher{}
	svc := NewTeamService(repo, newTestAuthorizer(t, editors), publisher)

	err := svc.CreateTeam(ctxAs("user-editor", RoleEditor), &domain.Team{Name: "New Club"})
	assert.ErrorIs(t, err, ErrForbidden)

	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	require.NoError(t, svc.CreateTeam(ctxAs("user-admin", RoleAdmin), &domain.Team{Name: "New Club"}))
	assert.True(t, publisher.published("football.team.created"))
}

func TestTeamService_ValidatesFoundedYear(t *testing.T) {
	repo := new(MockTeamRepository)
	svc := NewTeamService(repo, newTestAuthorizer(t, newStubEditors()), &recordingPublisher{})

	err := svc.CreateTeam(ctxAs("user-admin", RoleAdmin), &domain.Team{
		Name:        "Time Travellers FC",
		FoundedYear: ptr(3000),
	})

	assert.Error(t, err)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// --- player -------------------------------------------------------------

func TestPlayerService_CreatePlayer_RBAC(t *testing.T) {
	const editorID = "user-editor"

	t.Run("editor may add to a club they manage", func(t *testing.T) {
		editors := newStubEditors().grant(editorID, "team-1")
		repo := new(MockPlayerRepository)
		publisher := &recordingPublisher{}
		svc := NewPlayerService(repo, newTestAuthorizer(t, editors), publisher)

		player := &domain.Player{TeamID: ptr("team-1"), Name: "New Player", Position: "GK"}
		repo.On("Create", mock.Anything, player).Return(nil).Once()

		require.NoError(t, svc.CreatePlayer(ctxAs(editorID, RoleEditor), player))
		assert.True(t, publisher.published("football.player.created"))
	})

	t.Run("editor may not add to a club they do not manage", func(t *testing.T) {
		editors := newStubEditors().grant(editorID, "team-2")
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo, newTestAuthorizer(t, editors), &recordingPublisher{})

		player := &domain.Player{TeamID: ptr("team-1"), Name: "New Player", Position: "GK"}
		err := svc.CreatePlayer(ctxAs(editorID, RoleEditor), player)

		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("only an admin may create a free agent", func(t *testing.T) {
		// A player belonging to no club is covered by no club grant, so an
		// editor has nothing that could authorize it.
		editors := newStubEditors().grant(editorID, "team-1")
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo, newTestAuthorizer(t, editors), &recordingPublisher{})

		player := &domain.Player{TeamID: nil, Name: "Free Agent", Position: "ST"}
		assert.ErrorIs(t, svc.CreatePlayer(ctxAs(editorID, RoleEditor), player), ErrForbidden)

		repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
		assert.NoError(t, svc.CreatePlayer(ctxAs("user-admin", RoleAdmin), player))
	})
}

// TestPlayerService_UpdateCannotMoveClubOrValue guards the split between
// descriptive edits and derived state: a plain update must not be a back door
// for transferring a player or revaluing them without a recorded event.
func TestPlayerService_UpdateCannotMoveClubOrValue(t *testing.T) {
	editors := newStubEditors().grant("user-editor", "team-1")
	repo := new(MockPlayerRepository)
	svc := NewPlayerService(repo, newTestAuthorizer(t, editors), &recordingPublisher{})

	existing := &domain.Player{
		ID: "player-1", TeamID: ptr("team-1"), Name: "Old", Position: "ST",
		MarketValue: 1_000_000, Currency: "EUR",
	}
	repo.On("GetByID", mock.Anything, "player-1").Return(existing, nil).Once()

	var saved *domain.Player
	repo.On("Update", mock.Anything, mock.Anything).Return(nil).Once().
		Run(func(args mock.Arguments) { saved = args.Get(1).(*domain.Player) })

	// The caller attempts to move the player and inflate their value.
	err := svc.UpdatePlayer(ctxAs("user-editor", RoleEditor), &domain.Player{
		ID: "player-1", TeamID: ptr("team-99"), Name: "New", Position: "ST",
		MarketValue: 500_000_000,
	})

	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, "New", saved.Name, "descriptive fields should be applied")
	assert.Equal(t, "team-1", *saved.TeamID, "club must not change through an update")
	assert.Equal(t, domain.Minor(1_000_000), saved.MarketValue, "value must not change through an update")
}

func TestPlayerService_Validation(t *testing.T) {
	repo := new(MockPlayerRepository)
	svc := NewPlayerService(repo, newTestAuthorizer(t, newStubEditors()), &recordingPublisher{})
	ctx := ctxAs("user-admin", RoleAdmin)

	tests := []struct {
		name   string
		player *domain.Player
	}{
		{"missing name", &domain.Player{Position: "GK"}},
		{"missing position", &domain.Player{Name: "X"}},
		{"bad foot", &domain.Player{Name: "X", Position: "GK", PreferredFoot: ptr(domain.Foot("sideways"))}},
		{"implausible height", &domain.Player{Name: "X", Position: "GK", HeightCM: ptr(400)}},
		{"squad number out of range", &domain.Player{Name: "X", Position: "GK", SquadNumber: ptr(120)}},
		{"negative value", &domain.Player{Name: "X", Position: "GK", MarketValue: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Error(t, svc.CreatePlayer(ctx, tt.player))
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})
	}
}

// --- coach --------------------------------------------------------------

func TestCoachService_CreateCoach_RBAC(t *testing.T) {
	const editorID = "user-editor"

	t.Run("editor managing the club may add a coach", func(t *testing.T) {
		editors := newStubEditors().grant(editorID, "team-1")
		repo := new(MockCoachRepository)
		svc := NewCoachService(repo, newTestAuthorizer(t, editors))

		coach := &domain.Coach{TeamID: ptr("team-1"), Name: "New Coach"}
		repo.On("Create", mock.Anything, coach).Return(nil).Once()

		require.NoError(t, svc.CreateCoach(ctxAs(editorID, RoleEditor), coach))
		repo.AssertExpectations(t)
	})

	t.Run("editor not managing the club may not", func(t *testing.T) {
		editors := newStubEditors().grant(editorID, "team-2")
		repo := new(MockCoachRepository)
		svc := NewCoachService(repo, newTestAuthorizer(t, editors))

		err := svc.CreateCoach(ctxAs(editorID, RoleEditor), &domain.Coach{TeamID: ptr("team-1"), Name: "X"})
		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})
}

// --- deletes ------------------------------------------------------------

func TestServiceDeletes_AreAdminOnly(t *testing.T) {
	editors := newStubEditors().grant("user-editor", "team-1")
	authz := newTestAuthorizer(t, editors)
	editorCtx := ctxAs("user-editor", RoleEditor)
	adminCtx := ctxAs("user-admin", RoleAdmin)

	t.Run("team", func(t *testing.T) {
		repo := new(MockTeamRepository)
		svc := NewTeamService(repo, authz, &recordingPublisher{})

		assert.ErrorIs(t, svc.DeleteTeam(editorCtx, "team-1"), ErrForbidden)
		repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)

		repo.On("Delete", mock.Anything, "team-1").Return(nil).Once()
		assert.NoError(t, svc.DeleteTeam(adminCtx, "team-1"))
	})

	t.Run("player", func(t *testing.T) {
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo, authz, &recordingPublisher{})

		assert.ErrorIs(t, svc.DeletePlayer(editorCtx, "player-1"), ErrForbidden)
		repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)

		repo.On("Delete", mock.Anything, "player-1").Return(nil).Once()
		assert.NoError(t, svc.DeletePlayer(adminCtx, "player-1"))
	})
}
