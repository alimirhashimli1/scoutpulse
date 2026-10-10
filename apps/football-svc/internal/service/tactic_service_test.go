package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubTactics is an in-memory TacticRepository.
type stubTactics struct {
	mu     sync.Mutex
	boards map[string]domain.TacticBoard // by slug
}

func newStubTactics() *stubTactics {
	return &stubTactics{boards: map[string]domain.TacticBoard{}}
}

func (s *stubTactics) GetBySlug(_ context.Context, slug string) (*domain.TacticBoard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.boards[slug]
	if !ok {
		return nil, apperr.NotFound("tactics board not found")
	}
	return &b, nil
}

func (s *stubTactics) List(_ context.Context, ownerID *string, page domain.Page) ([]domain.TacticSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.TacticSummary
	for _, b := range s.boards {
		if ownerID != nil && b.OwnerID != *ownerID {
			continue
		}
		out = append(out, domain.TacticSummary{
			Slug: b.Slug, OwnerID: b.OwnerID, OwnerName: b.OwnerName, Title: b.Title,
			CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OwnerName != out[j].OwnerName {
			return strings.ToLower(out[i].OwnerName) < strings.ToLower(out[j].OwnerName)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *stubTactics) Create(_ context.Context, b *domain.TacticBoard) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, taken := s.boards[b.Slug]; taken {
		return apperr.Conflict("tactics board already exists")
	}
	b.ID = "id-" + b.Slug
	b.CreatedAt = time.Now()
	b.UpdatedAt = b.CreatedAt
	s.boards[b.Slug] = *b
	return nil
}

func (s *stubTactics) Update(_ context.Context, b *domain.TacticBoard) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.boards[b.Slug]; !ok {
		return apperr.NotFound("tactics board not found")
	}
	b.UpdatedAt = time.Now()
	s.boards[b.Slug] = *b
	return nil
}

func (s *stubTactics) Delete(_ context.Context, slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.boards[slug]; !ok {
		return apperr.NotFound("tactics board not found")
	}
	delete(s.boards, slug)
	return nil
}

func (s *stubTactics) DeleteByOwner(_ context.Context, ownerID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for slug, b := range s.boards {
		if b.OwnerID == ownerID {
			delete(s.boards, slug)
			n++
		}
	}
	return n, nil
}

var _ repository.TacticRepository = (*stubTactics)(nil)

// ctxAsNamed is ctxAs with a username, which tactics boards are filed under.
func ctxAsNamed(userID, username, role string) context.Context {
	return context.WithValue(context.Background(), auth.ClaimsContextKey, &auth.Claims{
		UserID: userID, Username: username, Role: role,
	})
}

func validBoard(title string) TacticInput {
	return TacticInput{Title: title, Content: json.RawMessage(`{"v":1,"steps":[{}]}`)}
}

func TestTactics_AnyMemberCanCreate_UnderTheirOwnName(t *testing.T) {
	svc := NewTacticService(newStubTactics())

	board, err := svc.Create(ctxAsNamed("u1", "aligsbsu", "user"), validBoard("  Long passes drill  "))

	require.NoError(t, err)
	assert.Equal(t, "Long passes drill", board.Title, "the title is trimmed")
	assert.Equal(t, "u1", board.OwnerID)
	assert.Equal(t, "aligsbsu", board.OwnerName, "the name comes from the token")
	assert.True(t, validSlug(board.Slug), "a slug is drawn: %q", board.Slug)
}

func TestTactics_CreateNeedsAnAccount(t *testing.T) {
	svc := NewTacticService(newStubTactics())

	_, err := svc.Create(context.Background(), validBoard("Pressing"))

	assert.Equal(t, apperr.KindUnauthorized, apperr.KindOf(err))
}

// The point of a board is to be sent to people without accounts.
func TestTactics_GetIsPublic(t *testing.T) {
	svc := NewTacticService(newStubTactics())
	created, err := svc.Create(ctxAsNamed("u1", "coach", "user"), validBoard("Build-up"))
	require.NoError(t, err)

	got, err := svc.Get(context.Background(), created.Slug)

	require.NoError(t, err)
	assert.Equal(t, "Build-up", got.Title)
	assert.JSONEq(t, `{"v":1,"steps":[{}]}`, string(got.Content))
}

// The directory is what keeps boards unlisted, so it is members only.
func TestTactics_ListNeedsAnAccount(t *testing.T) {
	svc := NewTacticService(newStubTactics())

	_, err := svc.List(context.Background(), nil, domain.NewPage(10, 0))

	assert.Equal(t, apperr.KindUnauthorized, apperr.KindOf(err))
}

func TestTactics_ListGroupsByAuthor(t *testing.T) {
	svc := NewTacticService(newStubTactics())
	_, err := svc.Create(ctxAsNamed("u2", "real123", "user"), validBoard("Atlético – Real analysis"))
	require.NoError(t, err)
	_, err = svc.Create(ctxAsNamed("u1", "aligsbsu", "user"), validBoard("Long passes drill"))
	require.NoError(t, err)

	list, err := svc.List(ctxAsNamed("u3", "viewer", "user"), nil, domain.NewPage(10, 0))

	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "aligsbsu", list[0].OwnerName)
	assert.Equal(t, "real123", list[1].OwnerName)
}

func TestTactics_OnlyTheAuthorCanEdit(t *testing.T) {
	svc := NewTacticService(newStubTactics())
	created, err := svc.Create(ctxAsNamed("u1", "aligsbsu", "user"), validBoard("Rondo"))
	require.NoError(t, err)

	_, err = svc.Update(ctxAsNamed("u2", "real123", "user"), created.Slug, validBoard("Mine now"))
	assert.Equal(t, apperr.KindForbidden, apperr.KindOf(err), "another member")

	_, err = svc.Update(ctxAsNamed("admin", "admin", RoleAdmin), created.Slug, validBoard("Moderated"))
	assert.Equal(t, apperr.KindForbidden, apperr.KindOf(err), "an administrator deletes, not edits")

	updated, err := svc.Update(ctxAsNamed("u1", "aligsbsu", "user"), created.Slug, validBoard("Rondo 4v2"))
	require.NoError(t, err)
	assert.Equal(t, "Rondo 4v2", updated.Title)
	assert.Equal(t, created.Slug, updated.Slug, "the link survives an edit")
}

func TestTactics_AuthorOrAdminCanDelete(t *testing.T) {
	svc := NewTacticService(newStubTactics())
	ctxAuthor := ctxAsNamed("u1", "aligsbsu", "user")
	first, err := svc.Create(ctxAuthor, validBoard("One"))
	require.NoError(t, err)
	second, err := svc.Create(ctxAuthor, validBoard("Two"))
	require.NoError(t, err)

	err = svc.Delete(ctxAsNamed("u2", "real123", RoleEditor), first.Slug)
	assert.Equal(t, apperr.KindForbidden, apperr.KindOf(err))

	require.NoError(t, svc.Delete(ctxAuthor, first.Slug))
	require.NoError(t, svc.Delete(ctxAsNamed("admin", "admin", RoleAdmin), second.Slug))
}

func TestTactics_Validation(t *testing.T) {
	svc := NewTacticService(newStubTactics())
	ctx := ctxAsNamed("u1", "coach", "user")

	cases := map[string]TacticInput{
		"blank title":      {Title: "   ", Content: json.RawMessage(`{}`)},
		"long title":       {Title: strings.Repeat("a", MaxTacticTitleLength+1), Content: json.RawMessage(`{}`)},
		"long description": {Title: "x", Description: strings.Repeat("a", MaxTacticDescriptionLength+1), Content: json.RawMessage(`{}`)},
		"no content":       {Title: "x"},
		"array content":    {Title: "x", Content: json.RawMessage(`[1,2]`)},
		"oversized":        {Title: "x", Content: json.RawMessage(`{"a":"` + strings.Repeat("a", MaxTacticContentBytes) + `"}`)},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Create(ctx, input)
			assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err))
		})
	}
}

// A malformed slug is answered without a lookup, as not-found rather than a
// bad request: from outside, there is simply no board at that address.
func TestTactics_MalformedSlugIsNotFound(t *testing.T) {
	svc := NewTacticService(newStubTactics())

	for _, slug := range []string{"", "short", "000000000000", "abcdefghijk!", "../../etc/pas"} {
		_, err := svc.Get(context.Background(), slug)
		assert.Equal(t, apperr.KindNotFound, apperr.KindOf(err), slug)
	}
}

func TestTactics_SlugCollisionIsRetried(t *testing.T) {
	repo := newStubTactics()
	svc := NewTacticService(repo).(*tacticService)
	ctx := ctxAsNamed("u1", "coach", "user")

	slugs := []string{"aaaaaaaaaaaa", "aaaaaaaaaaaa", "bbbbbbbbbbbb"}
	svc.newSlug = func() (string, error) {
		next := slugs[0]
		slugs = slugs[1:]
		return next, nil
	}

	first, err := svc.Create(ctx, validBoard("First"))
	require.NoError(t, err)
	second, err := svc.Create(ctx, validBoard("Second"))
	require.NoError(t, err)

	assert.Equal(t, "aaaaaaaaaaaa", first.Slug)
	assert.Equal(t, "bbbbbbbbbbbb", second.Slug, "the collision drew a fresh slug")
}

func TestUserDeleted_RemovesTheirTacticsBoards(t *testing.T) {
	editors := newStubEditors()
	tactics := newStubTactics()
	svc := NewTacticService(tactics)
	doomed, err := svc.Create(ctxAsNamed("doomed-user", "doomed", "user"), validBoard("Gone"))
	require.NoError(t, err)
	kept, err := svc.Create(ctxAsNamed("other-user", "other", "user"), validBoard("Kept"))
	require.NoError(t, err)

	consumer := NewIdentityEventConsumer(editors, tactics, newTestAuthorizer(t, editors), quietLogger())
	err = consumer.handleUserDeleted(context.Background(),
		envelopeFor(t, events.SubjectUserDeleted, events.UserDeleted{UserID: "doomed-user"}))
	require.NoError(t, err)

	_, err = tactics.GetBySlug(context.Background(), doomed.Slug)
	assert.Equal(t, apperr.KindNotFound, apperr.KindOf(err))
	_, err = tactics.GetBySlug(context.Background(), kept.Slug)
	assert.NoError(t, err)
}
