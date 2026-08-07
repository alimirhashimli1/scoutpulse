package handler

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/identity-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
	"golang.org/x/crypto/bcrypt"
)

// minPasswordLength is the shortest password accepted at registration.
const minPasswordLength = 8

// Handler handles authentication requests.
type Handler struct {
	UserRepo repository.UserRepository
}

// RegisterRequest is the payload for public self-registration.
//
// It deliberately has no Role field. Accepting a client-supplied role here
// would let anyone register as an admin and gain write access to the entire
// dataset. Every self-registered account gets model.UserRole; elevating an
// account is an administrative operation and belongs on its own endpoint.
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Identifier string `json:"identifier"` // Email or Username
	Password   string `json:"password"`
}

// LoginResponse is the successful login body.
type LoginResponse struct {
	Token string `json:"token"`
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Identity Service is healthy"))
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if req.Username == "" || req.Email == "" {
		httpx.WriteError(w, r, apperr.Invalid("username and email are required"))
		return
	}
	if len(req.Password) < minPasswordLength {
		httpx.WriteError(w, r, apperr.Invalid(
			fmt.Sprintf("password must be at least %d characters", minPasswordLength)))
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		httpx.WriteError(w, r, apperr.Internal(err))
		return
	}

	user := model.User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		// Self-registration never grants elevated privileges.
		Role:           model.UserRole,
		ManagedTeamIDs: []string{},
	}

	if err := h.UserRepo.Create(r.Context(), &user); err != nil {
		// The cause can carry SQL and constraint detail, so it is logged by
		// WriteError rather than returned to the caller.
		httpx.WriteError(w, r, apperr.Wrap(apperr.KindInternal, "failed to register user", err))
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, user)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// An unknown identifier and a wrong password return the same response, so
	// the endpoint cannot be used to enumerate accounts.
	user, err := h.UserRepo.GetByIdentifier(r.Context(), req.Identifier)
	if err != nil {
		httpx.WriteError(w, r, apperr.Unauthorized("invalid credentials"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		httpx.WriteError(w, r, apperr.Unauthorized("invalid credentials"))
		return
	}

	token, err := auth.GenerateToken(user.ID, string(user.Role), user.ManagedTeamIDs)
	if err != nil {
		httpx.WriteError(w, r, apperr.Wrap(apperr.KindInternal, "failed to issue token", err))
		return
	}

	httpx.WriteJSON(w, http.StatusOK, LoginResponse{Token: token})
}
