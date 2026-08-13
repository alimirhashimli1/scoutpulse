package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/libs/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func ctxAs(userID string, role model.Role) context.Context {
	return context.WithValue(context.Background(), auth.ClaimsContextKey,
		&auth.Claims{UserID: userID, Role: string(role)})
}

func changePassword(t *testing.T, h *Handler, ctx context.Context, body ChangePasswordRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/me/password", bytes.NewReader(raw)).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)
	return rr
}

// TestChangePassword_EndsEverySession is the point of the endpoint.
//
// If the password is being changed because it leaked, leaving the existing
// sessions alive would defeat the exercise — including the caller's own.
func TestChangePassword_EndsEverySession(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.MinCost)
	require.NoError(t, err)

	repo := new(MockUserRepository)
	repo.On("GetByID", mock.Anything, "user-1").
		Return(&model.User{ID: "user-1", PasswordHash: string(hash)}, nil)
	repo.On("UpdatePassword", mock.Anything, "user-1", mock.Anything).Return(nil).Once()

	sessions := newFakeRefreshRepo()
	h := &Handler{UserRepo: repo, RefreshRepo: sessions}

	rr := changePassword(t, h, ctxAs("user-1", model.UserRole), ChangePasswordRequest{
		CurrentPassword: "old-password", NewPassword: "a-new-password",
	})

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Positive(t, sessions.revokeAll, "changing a password must end existing sessions")
	repo.AssertExpectations(t)
}

// TestChangePassword_RequiresTheCurrentOne: holding a valid token is not
// enough. A token can be taken from an unlocked machine; knowing the existing
// password is what proves ownership.
func TestChangePassword_RequiresTheCurrentOne(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.MinCost)
	require.NoError(t, err)

	repo := new(MockUserRepository)
	repo.On("GetByID", mock.Anything, "user-1").
		Return(&model.User{ID: "user-1", PasswordHash: string(hash)}, nil)

	sessions := newFakeRefreshRepo()
	h := &Handler{UserRepo: repo, RefreshRepo: sessions}

	rr := changePassword(t, h, ctxAs("user-1", model.UserRole), ChangePasswordRequest{
		CurrentPassword: "not-the-old-password", NewPassword: "a-new-password",
	})

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	repo.AssertNotCalled(t, "UpdatePassword", mock.Anything, mock.Anything, mock.Anything)
	assert.Zero(t, sessions.revokeAll, "a failed attempt must not end sessions")
}

func TestChangePassword_RejectsShortAndUnchanged(t *testing.T) {
	tests := []struct {
		name string
		body ChangePasswordRequest
	}{
		{"too short", ChangePasswordRequest{CurrentPassword: "old-password", NewPassword: "short"}},
		{"unchanged", ChangePasswordRequest{CurrentPassword: "old-password", NewPassword: "old-password"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockUserRepository)
			h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

			rr := changePassword(t, h, ctxAs("user-1", model.UserRole), tt.body)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			repo.AssertNotCalled(t, "UpdatePassword", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// TestChangePassword_ProviderAccountSaysSo: an account created through Google
// has no password. "Invalid credentials" would be misleading.
func TestChangePassword_ProviderAccountSaysSo(t *testing.T) {
	repo := new(MockUserRepository)
	repo.On("GetByID", mock.Anything, "user-1").
		Return(&model.User{ID: "user-1", PasswordHash: ""}, nil)

	h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

	rr := changePassword(t, h, ctxAs("user-1", model.UserRole), ChangePasswordRequest{
		CurrentPassword: "anything", NewPassword: "a-new-password",
	})

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "linked provider")
}

func TestListUsers_IsAdminOnly(t *testing.T) {
	repo := new(MockUserRepository)
	h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil).
		WithContext(ctxAs("user-1", model.UserRole))
	rr := httptest.NewRecorder()

	h.ListUsers(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	repo.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestListUsers_ClampsAndTrimsThePage mirrors the football service: an
// oversized limit is clamped, and the extra row fetched to detect another page
// is not returned to the caller.
func TestListUsers_ClampsAndTrimsThePage(t *testing.T) {
	many := make([]model.User, 0, maxUserPageSize+1)
	for i := 0; i < maxUserPageSize+1; i++ {
		many = append(many, model.User{ID: "u", Username: "u"})
	}

	repo := new(MockUserRepository)
	// limit+1 is requested so the sentinel row reveals has_more.
	repo.On("List", mock.Anything, "", maxUserPageSize+1, 0).Return(many, nil).Once()

	h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?limit=9999", nil).
		WithContext(ctxAs("admin-1", model.AdminRole))
	rr := httptest.NewRecorder()

	h.ListUsers(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var body UserListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, maxUserPageSize, body.Limit, "an oversized limit must be clamped")
	assert.Len(t, body.Items, maxUserPageSize, "the sentinel row must not be returned")
	assert.True(t, body.HasMore)
	repo.AssertExpectations(t)
}

func TestDeleteUser_IsAdminOnly(t *testing.T) {
	repo := new(MockUserRepository)
	h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/user-2", nil).
		WithContext(ctxAs("user-1", model.UserRole))
	req.SetPathValue("id", "user-2")
	rr := httptest.NewRecorder()

	h.DeleteUser(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

// TestDeleteUser_CannotDeleteSelf: an installation whose last administrator
// deletes their own account has no route back through the API.
func TestDeleteUser_CannotDeleteSelf(t *testing.T) {
	repo := new(MockUserRepository)
	h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/admin-1", nil).
		WithContext(ctxAs("admin-1", model.AdminRole))
	req.SetPathValue("id", "admin-1")
	rr := httptest.NewRecorder()

	h.DeleteUser(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestDeleteUser_Succeeds(t *testing.T) {
	repo := new(MockUserRepository)
	repo.On("GetByID", mock.Anything, "user-2").
		Return(&model.User{ID: "user-2", Username: "doomed"}, nil).Once()
	repo.On("Delete", mock.Anything, "user-2").Return(nil).Once()

	h := &Handler{UserRepo: repo, RefreshRepo: newFakeRefreshRepo()}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/user-2", nil).
		WithContext(ctxAs("admin-1", model.AdminRole))
	req.SetPathValue("id", "user-2")
	rr := httptest.NewRecorder()

	h.DeleteUser(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	repo.AssertExpectations(t)
}
