package handler

import (
	"fmt"
	"net/http"

	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
	"golang.org/x/crypto/bcrypt"
)

// Paging bounds for the administrative user list, matching the football
// service's so a client meets one convention rather than two.
const (
	defaultUserPageSize = 25
	maxUserPageSize     = 100
)

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// UserListResponse is the envelope the list endpoint returns, matching the
// shape every list in the football service uses.
type UserListResponse struct {
	Items   []model.User `json:"items"`
	Limit   int          `json:"limit"`
	Offset  int          `json:"offset"`
	HasMore bool         `json:"has_more"`
}

// ChangePassword replaces the caller's own password.
//
// The current password is required even though the caller already holds a
// valid token: a token can be borrowed from an unlocked machine, and knowing
// the existing password is what proves the person changing it is the owner.
//
// Every session is ended afterwards, including this one. That is the point —
// if the password is being changed because it leaked, leaving the sessions
// alive would defeat the exercise.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		httpx.WriteError(w, r, apperr.Unauthorized("authentication required"))
		return
	}

	var req ChangePasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if len(req.NewPassword) < minPasswordLength {
		httpx.WriteError(w, r, apperr.Invalid(
			fmt.Sprintf("new_password must be at least %d characters", minPasswordLength)))
		return
	}
	if req.NewPassword == req.CurrentPassword {
		httpx.WriteError(w, r, apperr.Invalid("new_password must differ from the current one"))
		return
	}

	user, err := h.UserRepo.GetByID(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// An account created through an OAuth provider has no usable password, so
	// there is nothing to verify against and nothing to replace. Say so
	// plainly rather than failing the comparison with "invalid credentials".
	if user.PasswordHash == "" {
		httpx.WriteError(w, r, apperr.Invalid(
			"this account signs in with a linked provider and has no password to change"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		httpx.WriteError(w, r, apperr.Unauthorized("current password is incorrect"))
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		httpx.WriteError(w, r, apperr.Internal(err))
		return
	}

	if err := h.UserRepo.UpdatePassword(r.Context(), user.ID, string(hashed)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err := h.RefreshRepo.RevokeAllForUser(r.Context(), user.ID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListUsers serves the account list. Administrator-only: who holds an account
// is not public information.
//
//	GET /api/v1/users?q=ali&limit=25&offset=0
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	limit, err := httpx.QueryInt(r, "limit", defaultUserPageSize)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	offset, err := httpx.QueryInt(r, "offset", 0)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if limit <= 0 {
		limit = defaultUserPageSize
	}
	if limit > maxUserPageSize {
		limit = maxUserPageSize
	}
	if offset < 0 {
		offset = 0
	}

	// One extra row reveals whether another page exists without a COUNT, the
	// same trick domain.Page uses in the football service.
	users, err := h.UserRepo.List(r.Context(), r.URL.Query().Get("q"), limit+1, offset)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}
	if users == nil {
		users = []model.User{}
	}

	httpx.WriteJSON(w, http.StatusOK, UserListResponse{
		Items: users, Limit: limit, Offset: offset, HasMore: hasMore,
	})
}

// DeleteUser removes an account.
//
// Administrator-only, and an administrator may not delete themselves: doing so
// through a mis-click could leave an installation with no administrator at
// all, and there is no route back from that through the API.
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		httpx.WriteError(w, r, apperr.Unauthorized("authentication required"))
		return
	}
	if err := h.requireAdmin(r); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	userID := r.PathValue("id")
	if userID == claims.UserID {
		httpx.WriteError(w, r, apperr.Invalid("an administrator cannot delete their own account"))
		return
	}

	// Read before deleting: the username is wanted in the event, and the row
	// is gone by the time consumers see it.
	user, err := h.UserRepo.GetByID(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err := h.UserRepo.Delete(r.Context(), userID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// The account's sessions cascade away with the row. Its editor grants live
	// in the football service's database, which no foreign key can reach, so
	// they are cleaned up by a consumer of this event.
	h.publishUserDeleted(r, userID, user.Username)

	w.WriteHeader(http.StatusNoContent)
}

// requireAdmin is the one authorization rule this service has.
func (h *Handler) requireAdmin(r *http.Request) error {
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		return apperr.Unauthorized("authentication required")
	}
	if claims.Role != string(model.AdminRole) {
		return apperr.Forbidden("administrator access required")
	}
	return nil
}
