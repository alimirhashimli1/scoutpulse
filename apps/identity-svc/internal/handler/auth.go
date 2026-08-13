package handler

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/scoutpulse/identity-svc/internal/model"
	"github.com/scoutpulse/identity-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/events"
	"github.com/scoutpulse/libs/platform/httpx"
	"golang.org/x/crypto/bcrypt"
)

// minPasswordLength is the shortest password accepted at registration.
const minPasswordLength = 8

// dummyPasswordHash is compared against when no account matches, so that the
// unknown-identifier path costs the same as the wrong-password path.
//
// It is generated once at startup from a random password rather than
// hardcoded: at bcrypt.DefaultCost it must match the cost of a real stored
// hash, and deriving it here keeps the two in step automatically if the cost
// is ever raised.
var dummyPasswordHash = mustGenerateDummyHash()

func mustGenerateDummyHash() []byte {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic("identity: generating dummy password hash: " + err.Error())
	}
	hash, err := bcrypt.GenerateFromPassword(secret, bcrypt.DefaultCost)
	if err != nil {
		panic("identity: generating dummy password hash: " + err.Error())
	}
	return hash
}

// Handler handles authentication requests.
type Handler struct {
	UserRepo    repository.UserRepository
	RefreshRepo repository.RefreshTokenRepository
	// Publisher announces account changes other services hold data against.
	// Optional: with no broker configured this is a no-op, and every endpoint
	// still works — events are a secondary effect of a write, not part of it.
	Publisher events.Publisher
	// OAuth carries the external sign-in providers. Optional: with no provider
	// credentials configured, the password endpoints work exactly as before
	// and the provider routes report that none is enabled.
	OAuth OAuthDeps
}

// publishUserDeleted tells the rest of the platform an account is gone.
//
// The football service keeps editor grants keyed by user id in its own
// database, where no foreign key can reach. Without this event those grants
// would outlive the account and, worse than being useless, would apply again
// if the id were ever reused.
func (h *Handler) publishUserDeleted(r *http.Request, userID, username string) {
	if h.Publisher == nil {
		return
	}
	// The row is already deleted, so a delivery failure must not be reported
	// to the caller as a failed deletion. SafePublisher logs and counts it.
	_ = h.Publisher.Publish(r.Context(), events.SubjectUserDeleted, events.UserDeleted{
		UserID:   userID,
		Username: username,
	})
}

// publishRoleChanged lets consumers drop cached authorization decisions at
// once rather than waiting out a TTL.
func (h *Handler) publishRoleChanged(r *http.Request, userID, oldRole, newRole string) {
	if h.Publisher == nil {
		return
	}
	_ = h.Publisher.Publish(r.Context(), events.SubjectUserRoleChanged, events.UserRoleChanged{
		UserID:  userID,
		OldRole: oldRole,
		NewRole: newRole,
	})
}

// RegisterRequest is the payload for public self-registration.
//
// It deliberately has no Role field. Accepting a client-supplied role here
// would let anyone register as an admin and gain write access to the entire
// dataset. Every self-registered account gets model.UserRole; elevating an
// account goes through UpdateRole, which is administrator-only.
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Identifier string `json:"identifier"` // email or username
	Password   string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// TokenResponse is returned by login and refresh.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	// ExpiresIn is the access token's lifetime in seconds, so a client knows
	// when to refresh without having to parse the token.
	ExpiresIn int `json:"expires_in"`
}

type UpdateRoleRequest struct {
	Role model.Role `json:"role"`
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
		Role: model.UserRole,
	}

	if err := h.UserRepo.Create(r.Context(), &user); err != nil {
		httpx.WriteError(w, r, err)
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

	// An unknown identifier and a wrong password return the same response and
	// take the same time, so the endpoint cannot be used to enumerate accounts
	// by either channel.
	//
	// Returning early on a missing user would skip the bcrypt comparison
	// entirely -- roughly 60-100ms of work -- which makes "no such account"
	// and "wrong password" trivially distinguishable by timing however
	// identical the response bodies are. Hashing against a fixed dummy hash
	// keeps both paths on the same cost.
	user, err := h.UserRepo.GetByIdentifier(r.Context(), req.Identifier)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(req.Password))
		httpx.WriteError(w, r, apperr.Unauthorized("invalid credentials"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		httpx.WriteError(w, r, apperr.Unauthorized("invalid credentials"))
		return
	}

	tokens, err := h.issueTokens(r, user)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, tokens)
}

// Refresh exchanges a refresh token for a new pair, rotating the refresh token.
//
// Rotation gives leak detection: a refresh token is single-use, so seeing one
// that has already been exchanged means a copy is circulating. The response to
// that is to end every session the user holds rather than to keep serving both
// the legitimate client and the attacker.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if req.RefreshToken == "" {
		httpx.WriteError(w, r, apperr.Invalid("refresh_token is required"))
		return
	}

	stored, err := h.RefreshRepo.GetByToken(r.Context(), req.RefreshToken)
	if err != nil {
		httpx.WriteError(w, r, apperr.Unauthorized("invalid refresh token"))
		return
	}

	if !stored.Active(time.Now()) {
		// A revoked token being presented means it leaked after rotation.
		if stored.RevokedAt != nil {
			_ = h.RefreshRepo.RevokeAllForUser(r.Context(), stored.UserID)
		}
		httpx.WriteError(w, r, apperr.Unauthorized("invalid refresh token"))
		return
	}

	user, err := h.UserRepo.GetByID(r.Context(), stored.UserID)
	if err != nil {
		httpx.WriteError(w, r, apperr.Unauthorized("invalid refresh token"))
		return
	}

	nextToken, err := auth.NewRefreshToken()
	if err != nil {
		httpx.WriteError(w, r, apperr.Internal(err))
		return
	}

	next := &model.RefreshToken{
		UserID:    user.ID,
		TokenHash: repository.HashToken(nextToken),
		ExpiresAt: time.Now().Add(auth.RefreshTokenTTL),
		UserAgent: userAgent(r),
	}
	if err := h.RefreshRepo.Rotate(r.Context(), stored.ID, next); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// The role is re-read from the database on every refresh, so a role
	// change takes effect within one access-token lifetime rather than
	// persisting until the user happens to log in again.
	accessToken, err := auth.GenerateToken(user.ID, string(user.Role))
	if err != nil {
		httpx.WriteError(w, r, apperr.Wrap(apperr.KindInternal, "failed to issue token", err))
		return
	}

	httpx.WriteJSON(w, http.StatusOK, TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: nextToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.AccessTokenTTL.Seconds()),
	})
}

// Logout revokes a refresh token. It always reports success: telling a caller
// whether the token existed would leak that information to anyone guessing.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if req.RefreshToken != "" {
		if err := h.RefreshRepo.Revoke(r.Context(), req.RefreshToken); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateRole changes a user's role. This is the administrative counterpart to
// registration refusing to accept one.
func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		httpx.WriteError(w, r, apperr.Unauthorized("authentication required"))
		return
	}
	if claims.Role != string(model.AdminRole) {
		httpx.WriteError(w, r, apperr.Forbidden("administrator access required"))
		return
	}

	var req UpdateRoleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !model.ValidRole(req.Role) {
		httpx.WriteError(w, r, apperr.Invalid("role must be one of: admin, editor, user"))
		return
	}

	userID := r.PathValue("id")

	// Read the old role first, so the event can carry what actually changed.
	// A consumer caching authorization decisions needs the transition, not
	// just the destination.
	existing, err := h.UserRepo.GetByID(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err := h.UserRepo.UpdateRole(r.Context(), userID, req.Role, claims.UserID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	h.publishRoleChanged(r, userID, string(existing.Role), string(req.Role))

	// The old role is baked into any token already issued, so end the user's
	// sessions and make them obtain a token carrying the new one. Without
	// this, a demoted administrator would keep their privileges until their
	// refresh token expired.
	if err := h.RefreshRepo.RevokeAllForUser(r.Context(), userID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": userID, "role": req.Role})
}

// Me returns the authenticated caller's account.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		httpx.WriteError(w, r, apperr.Unauthorized("authentication required"))
		return
	}

	user, err := h.UserRepo.GetByID(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, user)
}

// issueTokens mints an access/refresh pair and records the session.
func (h *Handler) issueTokens(r *http.Request, user *model.User) (TokenResponse, error) {
	accessToken, err := auth.GenerateToken(user.ID, string(user.Role))
	if err != nil {
		return TokenResponse{}, apperr.Wrap(apperr.KindInternal, "failed to issue token", err)
	}

	refreshToken, err := auth.NewRefreshToken()
	if err != nil {
		return TokenResponse{}, apperr.Internal(err)
	}

	record := &model.RefreshToken{
		UserID:    user.ID,
		TokenHash: repository.HashToken(refreshToken),
		ExpiresAt: time.Now().Add(auth.RefreshTokenTTL),
		UserAgent: userAgent(r),
	}
	if err := h.RefreshRepo.Create(r.Context(), record); err != nil {
		return TokenResponse{}, err
	}

	return TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.AccessTokenTTL.Seconds()),
	}, nil
}

func userAgent(r *http.Request) *string {
	if ua := r.Header.Get("User-Agent"); ua != "" {
		// Bound the stored value: the header is attacker-controlled and
		// unbounded.
		if len(ua) > 512 {
			ua = ua[:512]
		}
		return &ua
	}
	return nil
}
