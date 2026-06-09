package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock services
type MockTeamService struct {
	mock.Mock
}

func (m *MockTeamService) GetTeam(ctx context.Context, id string) (*domain.Team, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Team), args.Error(1)
}

func (m *MockTeamService) ListTeamsByLeague(ctx context.Context, leagueID string) ([]domain.Team, error) {
	args := m.Called(ctx, leagueID)
	return args.Get(0).([]domain.Team), args.Error(1)
}

func (m *MockTeamService) CreateTeam(ctx context.Context, team *domain.Team) error {
	args := m.Called(ctx, team)
	return args.Error(0)
}

func (m *MockTeamService) UpdateTeam(ctx context.Context, team *domain.Team) error {
	args := m.Called(ctx, team)
	return args.Error(0)
}

func (m *MockTeamService) DeleteTeam(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestRBAC_CreateLeague(t *testing.T) {
	mockService := new(MockLeagueService)
	h := NewLeagueHandler(mockService)

	tests := []struct {
		name           string
		role           string
		expectedStatus int
	}{
		{"Admin should be allowed", "admin", http.StatusCreated},
		{"Editor should be forbidden", "editor", http.StatusForbidden},
		{"User should be forbidden", "user", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, _ := auth.GenerateToken("user-1", tt.role, nil)
			
			league := domain.League{Name: "Test League"}
			body, _ := json.Marshal(league)
			
			req := httptest.NewRequest("POST", "/leagues", bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer "+token)

			if tt.expectedStatus == http.StatusCreated {
				mockService.On("CreateLeague", mock.Anything, mock.Anything).Return(nil).Once()
			}

			rr := httptest.NewRecorder()
			handler := auth.AuthMiddleware(http.HandlerFunc(h.CreateLeague))
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			mockService.AssertExpectations(t)
		})
	}
}

func TestRBAC_CreateTeam(t *testing.T) {
	mockService := new(MockTeamService)
	h := NewTeamHandler(mockService)

	tests := []struct {
		name           string
		role           string
		expectedStatus int
	}{
		{"Admin should be allowed", "admin", http.StatusCreated},
		{"Editor should be forbidden", "editor", http.StatusForbidden},
		{"User should be forbidden", "user", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, _ := auth.GenerateToken("user-1", tt.role, nil)
			
			team := domain.Team{Name: "Test Team"}
			body, _ := json.Marshal(team)
			
			req := httptest.NewRequest("POST", "/teams", bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer "+token)

			if tt.expectedStatus == http.StatusCreated {
				mockService.On("CreateTeam", mock.Anything, mock.Anything).Return(nil).Once()
			}

			rr := httptest.NewRecorder()
			handler := auth.AuthMiddleware(http.HandlerFunc(h.CreateTeam))
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			mockService.AssertExpectations(t)
		})
	}
}

type MockLeagueService struct {
	mock.Mock
}

func (m *MockLeagueService) GetLeague(ctx context.Context, id string) (*domain.League, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.League), args.Error(1)
}

func (m *MockLeagueService) ListLeagues(ctx context.Context) ([]domain.League, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.League), args.Error(1)
}

func (m *MockLeagueService) CreateLeague(ctx context.Context, league *domain.League) error {
	args := m.Called(ctx, league)
	return args.Error(0)
}

func (m *MockLeagueService) UpdateLeague(ctx context.Context, league *domain.League) error {
	args := m.Called(ctx, league)
	return args.Error(0)
}

func (m *MockLeagueService) DeleteLeague(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

type MockCoachService struct {
	mock.Mock
}

func (m *MockCoachService) GetCoachByTeam(ctx context.Context, teamID string) (*domain.Coach, error) {
	args := m.Called(ctx, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Coach), args.Error(1)
}

func (m *MockCoachService) GetCoach(ctx context.Context, id string) (*domain.Coach, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Coach), args.Error(1)
}

func (m *MockCoachService) CreateCoach(ctx context.Context, coach *domain.Coach) error {
	args := m.Called(ctx, coach)
	return args.Error(0)
}

func (m *MockCoachService) UpdateCoach(ctx context.Context, coach *domain.Coach) error {
	args := m.Called(ctx, coach)
	return args.Error(0)
}

func (m *MockCoachService) DeleteCoach(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestRBAC_UpdateTeam(t *testing.T) {
	mockService := new(MockTeamService)
	h := NewTeamHandler(mockService)

	tests := []struct {
		name           string
		role           string
		managedTeamIDs []string
		teamID         string
		expectedStatus int
	}{
		{"Admin should be allowed", "admin", nil, "team-1", http.StatusOK},
		{"Editor managing team should be allowed", "editor", []string{"team-1"}, "team-1", http.StatusOK},
		{"Editor NOT managing team should be forbidden", "editor", []string{"team-2"}, "team-1", http.StatusForbidden},
		{"User should be forbidden", "user", nil, "team-1", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, _ := auth.GenerateToken("user-1", tt.role, tt.managedTeamIDs)
			
			team := domain.Team{ID: tt.teamID, Name: "Test Team"}
			body, _ := json.Marshal(team)
			
			req := httptest.NewRequest("PUT", "/teams/"+tt.teamID, bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer "+token)
			// Using net/http's PathValue requires Go 1.22+ and usually set by the mux
			// For testing, we can set it manually if we use the mux or just mock the request
			// But since we use r.PathValue("id"), we should use a mux or set it manually.
			
			// Set path value manually (Go 1.22 feature)
			req.SetPathValue("id", tt.teamID)

			if tt.expectedStatus == http.StatusOK {
				mockService.On("UpdateTeam", mock.Anything, mock.Anything).Return(nil).Once()
			}

			rr := httptest.NewRecorder()
			handler := auth.AuthMiddleware(http.HandlerFunc(h.UpdateTeam))
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			mockService.AssertExpectations(t)
		})
	}
}

func TestRBAC_CreateCoach(t *testing.T) {
	mockService := new(MockCoachService)
	h := NewCoachHandler(mockService)

	tests := []struct {
		name           string
		role           string
		managedTeamIDs []string
		teamID         string
		expectedStatus int
	}{
		{"Admin should be allowed", "admin", nil, "team-1", http.StatusCreated},
		{"Editor managing team should be allowed", "editor", []string{"team-1"}, "team-1", http.StatusCreated},
		{"Editor NOT managing team should be forbidden", "editor", []string{"team-2"}, "team-1", http.StatusForbidden},
		{"User should be forbidden", "user", nil, "team-1", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, _ := auth.GenerateToken("user-1", tt.role, tt.managedTeamIDs)
			
			coach := domain.Coach{TeamID: tt.teamID, Name: "Test Coach"}
			body, _ := json.Marshal(coach)
			
			req := httptest.NewRequest("POST", "/coaches", bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer "+token)

			if tt.expectedStatus == http.StatusCreated {
				mockService.On("CreateCoach", mock.Anything, mock.Anything).Return(nil).Once()
			}

			rr := httptest.NewRecorder()
			handler := auth.AuthMiddleware(http.HandlerFunc(h.CreateCoach))
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			mockService.AssertExpectations(t)
		})
	}
}
