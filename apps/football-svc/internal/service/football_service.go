package service

import (
	"context"
	"sync"
	"time"

	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
)

// Roles recognised by this service.
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
)

// grantCacheTTL bounds how stale an editor's club grants may be.
//
// Grants used to live in the JWT, so a revocation took up to 24 hours to bite.
// They are now read from the database, and this cache keeps the hot path off
// the database without reintroducing a long stale window: a revocation takes
// effect within a few seconds rather than a day.
const grantCacheTTL = 5 * time.Second

// Authorizer owns every write rule in the football domain.
//
// It is the single place authorization is decided. Handlers do not repeat
// these checks: when the rules lived in both layers the two copies drifted.
type Authorizer struct {
	editors repository.TeamEditorRepository

	mu    sync.RWMutex
	cache map[string]cachedGrant
}

type cachedGrant struct {
	manages   bool
	expiresAt time.Time
}

func NewAuthorizer(editors repository.TeamEditorRepository) *Authorizer {
	return &Authorizer{
		editors: editors,
		cache:   make(map[string]cachedGrant),
	}
}

// claims pulls the caller's identity off the context.
func (a *Authorizer) claims(ctx context.Context) (*auth.Claims, error) {
	c, ok := auth.GetClaims(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	return c, nil
}

// managesTeam reports whether the user holds an editor grant for the club,
// consulting the short-TTL cache first.
func (a *Authorizer) managesTeam(ctx context.Context, userID, teamID string) (bool, error) {
	if userID == "" || teamID == "" {
		return false, nil
	}

	key := userID + "\x00" + teamID
	now := time.Now()

	a.mu.RLock()
	entry, ok := a.cache[key]
	a.mu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.manages, nil
	}

	manages, err := a.editors.Manages(ctx, userID, teamID)
	if err != nil {
		return false, err
	}

	a.mu.Lock()
	a.cache[key] = cachedGrant{manages: manages, expiresAt: now.Add(grantCacheTTL)}
	// The cache is unbounded in principle; sweep expired entries when it
	// grows past a size a normal workload would not reach.
	if len(a.cache) > 1024 {
		for k, v := range a.cache {
			if now.After(v.expiresAt) {
				delete(a.cache, k)
			}
		}
	}
	a.mu.Unlock()

	return manages, nil
}

// InvalidateUser drops a user's cached grants so a change takes effect at once
// rather than after the TTL.
func (a *Authorizer) InvalidateUser(userID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k := range a.cache {
		if len(k) > len(userID) && k[:len(userID)] == userID && k[len(userID)] == 0 {
			delete(a.cache, k)
		}
	}
}

// InvalidateTeam drops every cached grant for a club. Used when the club is
// deleted, which cascades its grants away without naming the users who held
// them.
func (a *Authorizer) InvalidateTeam(teamID string) {
	suffix := "\x00" + teamID
	a.mu.Lock()
	defer a.mu.Unlock()
	for k := range a.cache {
		if len(k) > len(suffix) && k[len(k)-len(suffix):] == suffix {
			delete(a.cache, k)
		}
	}
}

// RequireAdmin allows only administrators.
func (a *Authorizer) RequireAdmin(ctx context.Context) error {
	claims, err := a.claims(ctx)
	if err != nil {
		return err
	}
	if claims.Role != RoleAdmin {
		return ErrForbidden
	}
	return nil
}

// RequireAuthenticated allows any caller holding a valid token.
func (a *Authorizer) RequireAuthenticated(ctx context.Context) error {
	_, err := a.claims(ctx)
	return err
}

// RequireTeam allows administrators, and editors holding a grant for the club.
func (a *Authorizer) RequireTeam(ctx context.Context, teamID string) error {
	claims, err := a.claims(ctx)
	if err != nil {
		return err
	}
	if claims.Role == RoleAdmin {
		return nil
	}
	if claims.Role != RoleEditor {
		return ErrForbidden
	}

	manages, err := a.managesTeam(ctx, claims.UserID, teamID)
	if err != nil {
		return err
	}
	if !manages {
		return ErrForbidden
	}
	return nil
}

// RequireTargetTeam allows administrators, and editors holding a grant for the
// destination club. A nil destination means the record belongs to no club --
// a free agent or a released coach -- which only an administrator may set,
// since no club grant can cover it.
func (a *Authorizer) RequireTargetTeam(ctx context.Context, teamID *string) error {
	if teamID == nil {
		return a.RequireAdmin(ctx)
	}
	return a.RequireTeam(ctx, *teamID)
}

// RequireEitherTeam allows the move when the caller manages either side of it.
//
// This is what makes transfers workable: the selling club's editor and the
// buying club's editor can both record the same move, and either releasing a
// player or signing one is authorized by the club the caller actually holds.
func (a *Authorizer) RequireEitherTeam(ctx context.Context, current, target *string) error {
	claims, err := a.claims(ctx)
	if err != nil {
		return err
	}
	if claims.Role == RoleAdmin {
		return nil
	}
	if claims.Role != RoleEditor {
		return ErrForbidden
	}

	for _, teamID := range []*string{current, target} {
		if teamID == nil {
			continue
		}
		manages, err := a.managesTeam(ctx, claims.UserID, *teamID)
		if err != nil {
			return err
		}
		if manages {
			return nil
		}
	}
	return ErrForbidden
}
