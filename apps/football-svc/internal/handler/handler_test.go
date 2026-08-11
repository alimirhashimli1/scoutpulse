package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestMain installs a key pair. Token operations fail closed without one, and
// these tests need to both mint and verify, so they hold the private key --
// the real service holds only the public half.
func TestMain(m *testing.M) {
	privatePEM, _, err := auth.GenerateKeyPair(auth.MinRSAKeyBits)
	if err != nil {
		panic(err)
	}
	if err := auth.SetSigningKey(privatePEM); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// --- mocks -------------------------------------------------------------

type MockTeamService struct{ mock.Mock }

func (m *MockTeamService) GetTeam(ctx context.Context, id string) (*domain.Team, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Team), args.Error(1)
}

func (m *MockTeamService) ListTeamsByLeague(ctx context.Context, leagueID string, page domain.Page) ([]domain.Team, error) {
	args := m.Called(ctx, leagueID, page)
	return args.Get(0).([]domain.Team), args.Error(1)
}

func (m *MockTeamService) CreateTeam(ctx context.Context, team *domain.Team) error {
	return m.Called(ctx, team).Error(0)
}

func (m *MockTeamService) UpdateTeam(ctx context.Context, team *domain.Team) error {
	return m.Called(ctx, team).Error(0)
}

func (m *MockTeamService) DeleteTeam(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

type MockPlayerService struct{ mock.Mock }

func (m *MockPlayerService) GetPlayer(ctx context.Context, id string) (*domain.Player, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Player), args.Error(1)
}

func (m *MockPlayerService) ListPlayersByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.Player, error) {
	args := m.Called(ctx, teamID, page)
	return args.Get(0).([]domain.Player), args.Error(1)
}

func (m *MockPlayerService) ListPlayers(ctx context.Context, filter repository.PlayerFilter, page domain.Page) ([]domain.Player, error) {
	args := m.Called(ctx, filter, page)
	return args.Get(0).([]domain.Player), args.Error(1)
}

func (m *MockPlayerService) CreatePlayer(ctx context.Context, player *domain.Player) error {
	return m.Called(ctx, player).Error(0)
}

func (m *MockPlayerService) UpdatePlayer(ctx context.Context, player *domain.Player) error {
	return m.Called(ctx, player).Error(0)
}

func (m *MockPlayerService) DeletePlayer(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

type MockLeagueService struct{ mock.Mock }

func (m *MockLeagueService) GetLeague(ctx context.Context, id string) (*domain.League, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.League), args.Error(1)
}

func (m *MockLeagueService) ListLeagues(ctx context.Context, page domain.Page) ([]domain.League, error) {
	args := m.Called(ctx, page)
	return args.Get(0).([]domain.League), args.Error(1)
}

func (m *MockLeagueService) CreateLeague(ctx context.Context, league *domain.League) error {
	return m.Called(ctx, league).Error(0)
}

func (m *MockLeagueService) UpdateLeague(ctx context.Context, league *domain.League) error {
	return m.Called(ctx, league).Error(0)
}

func (m *MockLeagueService) DeleteLeague(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

type MockCoachService struct{ mock.Mock }

func (m *MockCoachService) GetCoach(ctx context.Context, id string) (*domain.Coach, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Coach), args.Error(1)
}

func (m *MockCoachService) GetCoachByTeam(ctx context.Context, teamID string) (*domain.Coach, error) {
	args := m.Called(ctx, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Coach), args.Error(1)
}

func (m *MockCoachService) CreateCoach(ctx context.Context, coach *domain.Coach) error {
	return m.Called(ctx, coach).Error(0)
}

func (m *MockCoachService) UpdateCoach(ctx context.Context, coach *domain.Coach) error {
	return m.Called(ctx, coach).Error(0)
}

func (m *MockCoachService) DeleteCoach(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

// --- tests -------------------------------------------------------------

// TestHandlerMapsServiceErrors covers the contract that replaced the RBAC
// checks previously duplicated in each handler: authorization is decided by
// the service, and the handler's only job is to render the outcome with the
// right status. Role-by-role coverage lives in internal/service.
func TestHandlerMapsServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{"forbidden", service.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"unauthorized", service.ErrUnauthorized, http.StatusUnauthorized, "unauthorized"},
		{"not found", service.ErrNotFound, http.StatusNotFound, "not_found"},
		{"conflict", apperr.Conflict("team already exists"), http.StatusConflict, "conflict"},
		{"success", nil, http.StatusCreated, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := new(MockTeamService)
			h := NewTeamHandler(svc, nil, nil)
			svc.On("CreateTeam", mock.Anything, mock.Anything).Return(tt.serviceErr).Once()

			body, _ := json.Marshal(domain.Team{Name: "Test Team"})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/teams", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			h.CreateTeam(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)
			if tt.wantCode != "" {
				var resp httpx.ErrorResponse
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantCode, resp.Code)
			}
			svc.AssertExpectations(t)
		})
	}
}

// TestHandlerDoesNotLeakInternalErrors guards the fix for handlers that used
// to return err.Error() verbatim, exposing driver and SQL detail to callers.
func TestHandlerDoesNotLeakInternalErrors(t *testing.T) {
	svc := new(MockTeamService)
	h := NewTeamHandler(svc, nil, nil)

	leaky := apperr.Internal(assertableError("pq: duplicate key value violates unique constraint \"teams_name_key\""))
	svc.On("CreateTeam", mock.Anything, mock.Anything).Return(leaky).Once()

	body, _ := json.Marshal(domain.Team{Name: "Test Team"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teams", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.CreateTeam(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.NotContains(t, rr.Body.String(), "teams_name_key")
	assert.NotContains(t, rr.Body.String(), "pq:")
}

type assertableError string

func (e assertableError) Error() string { return string(e) }

// TestUpdateTeamUsesPathID ensures a body cannot redirect a write to another
// row: the service authorizes against the id it receives, so that id must
// come from the URL.
func TestUpdateTeamUsesPathID(t *testing.T) {
	svc := new(MockTeamService)
	h := NewTeamHandler(svc, nil, nil)

	var received *domain.Team
	svc.On("UpdateTeam", mock.Anything, mock.Anything).
		Return(nil).
		Once().
		Run(func(args mock.Arguments) { received = args.Get(1).(*domain.Team) })

	// Body claims a different team than the path.
	body, _ := json.Marshal(domain.Team{ID: "team-attacker", Name: "Renamed"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/teams/team-1", bytes.NewReader(body))
	req.SetPathValue("id", "team-1")
	rr := httptest.NewRecorder()

	h.UpdateTeam(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, received)
	assert.Equal(t, "team-1", received.ID)
}

func TestProtectedRouteRequiresToken(t *testing.T) {
	svc := new(MockTeamService)
	h := NewTeamHandler(svc, nil, nil)
	protected := auth.AuthMiddleware(http.HandlerFunc(h.CreateTeam))

	t.Run("missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams", bytes.NewReader([]byte(`{}`)))
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("malformed token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Authorization", "Bearer not-a-token")
		rr := httptest.NewRecorder()
		protected.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	svc.AssertNotCalled(t, "CreateTeam", mock.Anything, mock.Anything)
}

func TestListPlayersPaging(t *testing.T) {
	t.Run("defaults applied when unspecified", func(t *testing.T) {
		svc := new(MockPlayerService)
		h := NewPlayerHandler(svc)

		wantPage := domain.NewPage(domain.DefaultPageSize, 0)
		svc.On("ListPlayers", mock.Anything, mock.Anything, wantPage).
			Return([]domain.Player{}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/players", nil)
		rr := httptest.NewRecorder()
		h.ListPlayers(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		svc.AssertExpectations(t)
	})

	t.Run("oversized limit is clamped", func(t *testing.T) {
		svc := new(MockPlayerService)
		h := NewPlayerHandler(svc)

		wantPage := domain.NewPage(domain.MaxPageSize, 0)
		svc.On("ListPlayers", mock.Anything, mock.Anything, wantPage).
			Return([]domain.Player{}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/players?limit=100000", nil)
		rr := httptest.NewRecorder()
		h.ListPlayers(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		svc.AssertExpectations(t)
	})

	t.Run("malformed limit is rejected", func(t *testing.T) {
		svc := new(MockPlayerService)
		h := NewPlayerHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/players?limit=abc", nil)
		rr := httptest.NewRecorder()
		h.ListPlayers(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		svc.AssertNotCalled(t, "ListPlayers", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("response reports has_more when another page exists", func(t *testing.T) {
		svc := new(MockPlayerService)
		h := NewPlayerHandler(svc)

		// The repository returns limit+1 rows; the extra row is the signal.
		page := domain.NewPage(2, 0)
		rows := []domain.Player{{Name: "A"}, {Name: "B"}, {Name: "C"}}
		svc.On("ListPlayers", mock.Anything, mock.Anything, page).Return(rows, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/players?limit=2", nil)
		rr := httptest.NewRecorder()
		h.ListPlayers(rr, req)

		var resp domain.ListResult[domain.Player]
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Len(t, resp.Items, 2)
		assert.True(t, resp.HasMore)
		assert.Equal(t, 2, resp.Limit)
	})
}

func TestListPlayersFilters(t *testing.T) {
	svc := new(MockPlayerService)
	h := NewPlayerHandler(svc)

	var got repository.PlayerFilter
	svc.On("ListPlayers", mock.Anything, mock.Anything, mock.Anything).
		Return([]domain.Player{}, nil).
		Once().
		Run(func(args mock.Arguments) { got = args.Get(1).(repository.PlayerFilter) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/players?free_agent=true&position=GK", nil)
	rr := httptest.NewRecorder()
	h.ListPlayers(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, got.FreeAgent)
	assert.True(t, *got.FreeAgent)
	require.NotNil(t, got.Position)
	assert.Equal(t, "GK", *got.Position)
	assert.Nil(t, got.TeamID)
}

func TestGetTeam_MissingCoachIsNotAnError(t *testing.T) {
	teamSvc := new(MockTeamService)
	playerSvc := new(MockPlayerService)
	coachSvc := new(MockCoachService)
	h := NewTeamHandler(teamSvc, playerSvc, coachSvc)

	teamID := "team-1"
	teamSvc.On("GetTeam", mock.Anything, teamID).
		Return(&domain.Team{ID: teamID, Name: "Test Team"}, nil).Once()
	playerSvc.On("ListPlayersByTeam", mock.Anything, teamID, mock.Anything).
		Return([]domain.Player{}, nil).Once()
	coachSvc.On("GetCoachByTeam", mock.Anything, teamID).
		Return(nil, service.ErrNotFound).Once()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+teamID, nil)
	req.SetPathValue("id", teamID)
	rr := httptest.NewRecorder()

	h.GetTeam(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp TeamDetailResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Nil(t, resp.Coach)
	assert.Equal(t, teamID, resp.ID)

	teamSvc.AssertExpectations(t)
	playerSvc.AssertExpectations(t)
	coachSvc.AssertExpectations(t)
}

func TestGetTeam_NotFound(t *testing.T) {
	teamSvc := new(MockTeamService)
	h := NewTeamHandler(teamSvc, new(MockPlayerService), new(MockCoachService))

	teamSvc.On("GetTeam", mock.Anything, "missing").Return(nil, service.ErrNotFound).Once()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/missing", nil)
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()

	h.GetTeam(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}
