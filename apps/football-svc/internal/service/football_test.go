package service

import (
	"context"
	"testing"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTeamRepository is a mock of repository.TeamRepository
type MockTeamRepository struct {
	mock.Mock
}

func (m *MockTeamRepository) GetByID(ctx context.Context, id string) (*domain.Team, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Team), args.Error(1)
}

func (m *MockTeamRepository) ListByLeague(ctx context.Context, leagueID string, page domain.Page) ([]domain.Team, error) {
	args := m.Called(ctx, leagueID, page)
	return args.Get(0).([]domain.Team), args.Error(1)
}

func (m *MockTeamRepository) Create(ctx context.Context, team *domain.Team) error {
	args := m.Called(ctx, team)
	return args.Error(0)
}

func (m *MockTeamRepository) Update(ctx context.Context, team *domain.Team) error {
	args := m.Called(ctx, team)
	return args.Error(0)
}

func (m *MockTeamRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestTeamService_UpdateTeam_RBAC(t *testing.T) {
	repo := new(MockTeamRepository)
	svc := NewTeamService(repo)

	t.Run("Editor trying to modify an unassigned team should trigger Forbidden error", func(t *testing.T) {
		// Setup context with Editor claims but no management of team-1
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-2"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		team := &domain.Team{ID: "team-1", Name: "New Name"}

		// Act
		err := svc.UpdateTeam(ctx, team)

		// Assert
		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("Editor modifying assigned team should be allowed", func(t *testing.T) {
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-1"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		team := &domain.Team{ID: "team-1", Name: "New Name"}
		repo.On("Update", mock.Anything, team).Return(nil).Once()

		// Act
		err := svc.UpdateTeam(ctx, team)

		// Assert
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("Admin should be allowed to modify any team", func(t *testing.T) {
		claims := &auth.Claims{
			Role: "admin",
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		team := &domain.Team{ID: "team-1", Name: "New Name"}
		repo.On("Update", mock.Anything, team).Return(nil).Once()

		// Act
		err := svc.UpdateTeam(ctx, team)

		// Assert
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})
}

func TestPlayerService_UpdatePlayer_RBAC(t *testing.T) {
	t.Run("Editor trying to update a player of an unmanaged team should trigger Forbidden error", func(t *testing.T) {
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo)
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-2"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		teamID := "team-1"
		player := &domain.Player{ID: "player-1", TeamID: &teamID, Name: "Updated Player", MarketValue: 120.0}
		existingPlayer := &domain.Player{ID: "player-1", TeamID: &teamID, Name: "Existing Player", MarketValue: 100.0}
		repo.On("GetByID", mock.Anything, "player-1").Return(existingPlayer, nil).Once()

		// Act
		err := svc.UpdatePlayer(ctx, player)

		// Assert
		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("Editor updating a player on a managed current team should be allowed", func(t *testing.T) {
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo)
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-1"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		teamID := "team-1"
		player := &domain.Player{ID: "player-1", TeamID: &teamID, Name: "Updated Player", MarketValue: 120.0}
		existingPlayer := &domain.Player{ID: "player-1", TeamID: &teamID, Name: "Existing Player", MarketValue: 100.0}
		repo.On("GetByID", mock.Anything, "player-1").Return(existingPlayer, nil).Once()
		repo.On("Update", mock.Anything, player).Return(nil).Once()

		// Act
		err := svc.UpdatePlayer(ctx, player)

		// Assert
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("Editor transferring a player to a managed target team should be allowed", func(t *testing.T) {
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo)
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-2"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		currentTeamID := "team-1"
		targetTeamID := "team-2"
		player := &domain.Player{ID: "player-1", TeamID: &targetTeamID, Name: "Updated Player", MarketValue: 120.0}
		existingPlayer := &domain.Player{ID: "player-1", TeamID: &currentTeamID, Name: "Existing Player", MarketValue: 100.0}
		repo.On("GetByID", mock.Anything, "player-1").Return(existingPlayer, nil).Once()
		repo.On("Update", mock.Anything, player).Return(nil).Once()

		// Act
		err := svc.UpdatePlayer(ctx, player)

		// Assert
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("Admin updating any player should be allowed", func(t *testing.T) {
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo)
		claims := &auth.Claims{
			Role: "admin",
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		teamID := "team-1"
		player := &domain.Player{ID: "player-1", TeamID: &teamID, Name: "Updated Player", MarketValue: 120.0}
		existingPlayer := &domain.Player{ID: "player-1", TeamID: nil, Name: "Existing Player", MarketValue: 100.0}
		repo.On("GetByID", mock.Anything, "player-1").Return(existingPlayer, nil).Once()
		repo.On("Update", mock.Anything, player).Return(nil).Once()

		// Act
		err := svc.UpdatePlayer(ctx, player)

		// Assert
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("Editor releasing a player from a managed current team should be allowed", func(t *testing.T) {
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo)
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-1"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		player := &domain.Player{ID: "player-1", TeamID: nil, Name: "Updated Player", MarketValue: 120.0}
		currentTeamID := "team-1"
		existingPlayer := &domain.Player{ID: "player-1", TeamID: &currentTeamID, Name: "Existing Player", MarketValue: 100.0}
		repo.On("GetByID", mock.Anything, "player-1").Return(existingPlayer, nil).Once()
		repo.On("Update", mock.Anything, player).Return(nil).Once()

		// Act
		err := svc.UpdatePlayer(ctx, player)

		// Assert
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("Editor updating an unmanaged free agent should be forbidden", func(t *testing.T) {
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo)
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-1"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		player := &domain.Player{ID: "player-1", TeamID: nil, Name: "Updated Player", MarketValue: 120.0}
		existingPlayer := &domain.Player{ID: "player-1", TeamID: nil, Name: "Existing Player", MarketValue: 100.0}
		repo.On("GetByID", mock.Anything, "player-1").Return(existingPlayer, nil).Once()

		// Act
		err := svc.UpdatePlayer(ctx, player)

		// Assert
		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("User updating a player should be forbidden", func(t *testing.T) {
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo)
		claims := &auth.Claims{Role: "user"}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		teamID := "team-1"
		player := &domain.Player{ID: "player-1", TeamID: &teamID, Name: "Updated Player", MarketValue: 120.0}
		existingPlayer := &domain.Player{ID: "player-1", TeamID: &teamID, Name: "Existing Player", MarketValue: 100.0}
		repo.On("GetByID", mock.Anything, "player-1").Return(existingPlayer, nil).Once()

		// Act
		err := svc.UpdatePlayer(ctx, player)

		// Assert
		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})
}

// MockCoachRepository is a mock of repository.CoachRepository
type MockCoachRepository struct {
	mock.Mock
}

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
	args := m.Called(ctx, coach)
	return args.Error(0)
}

func (m *MockCoachRepository) Update(ctx context.Context, coach *domain.Coach) error {
	args := m.Called(ctx, coach)
	return args.Error(0)
}

func (m *MockCoachRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestCoachService_CreateCoach_RBAC(t *testing.T) {
	repo := new(MockCoachRepository)
	svc := NewCoachService(repo)

	t.Run("Editor trying to add a coach to an unmanaged team should trigger Forbidden error", func(t *testing.T) {
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-2"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		teamID := "team-1"
		coach := &domain.Coach{TeamID: &teamID, Name: "New Coach"}

		// Act
		err := svc.CreateCoach(ctx, coach)

		// Assert
		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("Editor adding coach to managed team should be allowed", func(t *testing.T) {
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-1"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		teamID := "team-1"
		coach := &domain.Coach{TeamID: &teamID, Name: "New Coach"}
		repo.On("Create", mock.Anything, coach).Return(nil).Once()

		// Act
		err := svc.CreateCoach(ctx, coach)

		// Assert
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})
}

func TestCoachService_UpdateCoach_RBAC(t *testing.T) {
	repo := new(MockCoachRepository)
	svc := NewCoachService(repo)

	t.Run("Editor updating coach for managed target team should be allowed", func(t *testing.T) {
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-1"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		teamID := "team-1"
		coach := &domain.Coach{ID: "coach-1", TeamID: &teamID, Name: "Updated Coach"}
		repo.On("Update", mock.Anything, coach).Return(nil).Once()

		// Act
		err := svc.UpdateCoach(ctx, coach)

		// Assert
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("User updating coach should be forbidden", func(t *testing.T) {
		repo := new(MockCoachRepository)
		svc := NewCoachService(repo)
		claims := &auth.Claims{Role: "user"}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		teamID := "team-1"
		coach := &domain.Coach{ID: "coach-1", TeamID: &teamID, Name: "Updated Coach"}

		// Act
		err := svc.UpdateCoach(ctx, coach)

		// Assert
		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})
}

// MockPlayerRepository is a mock of repository.PlayerRepository
type MockPlayerRepository struct {
	mock.Mock
}

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
	args := m.Called(ctx, player)
	return args.Error(0)
}

func (m *MockPlayerRepository) Update(ctx context.Context, player *domain.Player) error {
	args := m.Called(ctx, player)
	return args.Error(0)
}

func (m *MockPlayerRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestPlayerService_CreatePlayer_RBAC(t *testing.T) {
	repo := new(MockPlayerRepository)
	svc := NewPlayerService(repo)

	t.Run("Editor trying to add a player to an unmanaged team should trigger Forbidden error", func(t *testing.T) {
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-2"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		teamID := "team-1"
		player := &domain.Player{TeamID: &teamID, Name: "New Player"}

		// Act
		err := svc.CreatePlayer(ctx, player)

		// Assert
		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("Editor adding player to managed team should be allowed", func(t *testing.T) {
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-1"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		teamID := "team-1"
		player := &domain.Player{TeamID: &teamID, Name: "New Player"}
		repo.On("Create", mock.Anything, player).Return(nil).Once()

		// Act
		err := svc.CreatePlayer(ctx, player)

		// Assert
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})
}

func TestServiceDeletes_RBAC(t *testing.T) {
	t.Run("Editor deleting team should be forbidden", func(t *testing.T) {
		repo := new(MockTeamRepository)
		svc := NewTeamService(repo)
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-1"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		// Act
		err := svc.DeleteTeam(ctx, "team-1")

		// Assert
		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	})

	t.Run("Editor deleting player should be forbidden", func(t *testing.T) {
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo)
		claims := &auth.Claims{
			Role:           "editor",
			ManagedTeamIDs: []string{"team-1"},
		}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		// Act
		err := svc.DeletePlayer(ctx, "player-1")

		// Assert
		assert.ErrorIs(t, err, ErrForbidden)
		repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	})

	t.Run("Admin deleting player should be allowed", func(t *testing.T) {
		repo := new(MockPlayerRepository)
		svc := NewPlayerService(repo)
		claims := &auth.Claims{Role: "admin"}
		ctx := context.WithValue(context.Background(), auth.ClaimsContextKey, claims)

		repo.On("Delete", mock.Anything, "player-1").Return(nil).Once()

		// Act
		err := svc.DeletePlayer(ctx, "player-1")

		// Assert
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})
}
