package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/events"
)

// stubEditors is an in-memory TeamEditorRepository.
//
// Grants used to be assertable by putting ids in the token; they now live in a
// repository, so tests need one they can drive.
type stubEditors struct {
	mu     sync.Mutex
	grants map[string]map[string]bool // userID -> teamID -> granted
	calls  int                        // how many times Manages hit the store
}

func newStubEditors() *stubEditors {
	return &stubEditors{grants: map[string]map[string]bool{}}
}

// grant records that userID may edit each of teamIDs.
func (s *stubEditors) grant(userID string, teamIDs ...string) *stubEditors {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.grants[userID] == nil {
		s.grants[userID] = map[string]bool{}
	}
	for _, id := range teamIDs {
		s.grants[userID][id] = true
	}
	return s
}

func (s *stubEditors) revoke(userID, teamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.grants[userID], teamID)
}

func (s *stubEditors) Manages(_ context.Context, userID, teamID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.grants[userID][teamID], nil
}

func (s *stubEditors) ListTeams(_ context.Context, userID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for id, ok := range s.grants[userID] {
		if ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (s *stubEditors) ListEditors(context.Context, string, domain.Page) ([]repository.TeamEditor, error) {
	return nil, nil
}

func (s *stubEditors) Grant(_ context.Context, userID, teamID string, _ *string) error {
	s.grant(userID, teamID)
	return nil
}

func (s *stubEditors) Revoke(_ context.Context, userID, teamID string) error {
	s.revoke(userID, teamID)
	return nil
}

func (s *stubEditors) RevokeAllForUser(_ context.Context, userID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := int64(len(s.grants[userID]))
	delete(s.grants, userID)
	return removed, nil
}

// recordingPublisher captures published events so tests can assert on them.
type recordingPublisher struct {
	mu       sync.Mutex
	Subjects []string
	Payloads []any
}

func (p *recordingPublisher) Publish(_ context.Context, subject string, payload any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Subjects = append(p.Subjects, subject)
	p.Payloads = append(p.Payloads, payload)
	return nil
}

func (p *recordingPublisher) Close() error { return nil }

func (p *recordingPublisher) published(subject string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.Subjects {
		if s == subject {
			return true
		}
	}
	return false
}

var _ events.Publisher = (*recordingPublisher)(nil)

// ctxAs builds a request context carrying the given identity.
func ctxAs(userID, role string) context.Context {
	return context.WithValue(context.Background(), auth.ClaimsContextKey, &auth.Claims{
		UserID: userID,
		Role:   role,
	})
}

// ptr is a shorthand for taking the address of a literal.
func ptr[T any](v T) *T { return &v }

// today is a fixed date for tests that need one.
func today() time.Time { return time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC) }

// newTestAuthorizer builds an Authorizer over a stub grant store.
func newTestAuthorizer(t *testing.T, editors *stubEditors) *Authorizer {
	t.Helper()
	return NewAuthorizer(editors)
}
