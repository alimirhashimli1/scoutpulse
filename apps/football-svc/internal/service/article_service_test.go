package service

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubArticles is an in-memory ArticleRepository.
type stubArticles struct {
	mu       sync.Mutex
	articles map[string]domain.Article // by id
	next     int
}

func newStubArticles() *stubArticles {
	return &stubArticles{articles: map[string]domain.Article{}}
}

func (s *stubArticles) GetByID(_ context.Context, id string) (*domain.Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.articles[id]
	if !ok {
		return nil, apperr.NotFound("article not found")
	}
	return &a, nil
}

func (s *stubArticles) GetLive(_ context.Context, slug string, now time.Time) (*domain.Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.articles {
		if a.Slug == slug && a.Live(now) {
			return &a, nil
		}
	}
	return nil, apperr.NotFound("article not found")
}

func summary(a domain.Article) domain.ArticleSummary {
	return domain.ArticleSummary{
		ID: a.ID, Slug: a.Slug, AuthorID: a.AuthorID, AuthorName: a.AuthorName, Title: a.Title,
		Tags: a.Tags, Status: a.Status, PublishAt: a.PublishAt, SubmittedAt: a.SubmittedAt,
		ReviewNote: a.ReviewNote, UpdatedAt: a.UpdatedAt,
	}
}

func (s *stubArticles) ListLive(_ context.Context, tag *string, now time.Time, _ domain.Page) ([]domain.ArticleSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.ArticleSummary
	for _, a := range s.articles {
		if !a.Live(now) {
			continue
		}
		if tag != nil && !contains(a.Tags, *tag) {
			continue
		}
		out = append(out, summary(a))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishAt.After(*out[j].PublishAt) })
	return out, nil
}

func (s *stubArticles) ListDesk(_ context.Context, f domain.ArticleDeskFilter, _ domain.Page) ([]domain.ArticleSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.ArticleSummary
	for _, a := range s.articles {
		if f.AuthorID != nil && a.AuthorID != *f.AuthorID {
			continue
		}
		if f.Status != nil && a.Status != *f.Status {
			continue
		}
		out = append(out, summary(a))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *stubArticles) Create(_ context.Context, a *domain.Article) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, other := range s.articles {
		if other.Slug == a.Slug {
			return apperr.Conflict("article already exists")
		}
	}
	s.next++
	a.ID = "article-" + strconv.Itoa(s.next)
	a.CreatedAt = time.Now()
	a.UpdatedAt = a.CreatedAt
	s.articles[a.ID] = *a
	return nil
}

func (s *stubArticles) Update(_ context.Context, a *domain.Article) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.articles[a.ID]; !ok {
		return apperr.NotFound("article not found")
	}
	a.UpdatedAt = time.Now()
	s.articles[a.ID] = *a
	return nil
}

func (s *stubArticles) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.articles[id]; !ok {
		return apperr.NotFound("article not found")
	}
	delete(s.articles, id)
	return nil
}

func (s *stubArticles) CountByAuthor(_ context.Context, authorID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, a := range s.articles {
		if a.AuthorID == authorID {
			n++
		}
	}
	return n, nil
}

func (s *stubArticles) DeleteUnpublishedByAuthor(_ context.Context, authorID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for id, a := range s.articles {
		if a.AuthorID == authorID && a.Status != domain.ArticlePublished {
			delete(s.articles, id)
			n++
		}
	}
	return n, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// --- fixtures --------------------------------------------------------------

const (
	writerID = "11111111-1111-1111-1111-111111111111"
	otherID  = "22222222-2222-2222-2222-222222222222"
	adminID  = "33333333-3333-3333-3333-333333333333"
)

var deskNow = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func asWriter(id, role, name string) context.Context {
	return context.WithValue(context.Background(), auth.ClaimsContextKey, &auth.Claims{
		UserID: id, Role: role, Username: name,
	})
}

var (
	editor = asWriter(writerID, RoleEditor, "Ana")
	other  = asWriter(otherID, RoleEditor, "Ben")
	admin  = asWriter(adminID, RoleAdmin, "Chief")
	member = asWriter("44444444-4444-4444-4444-444444444444", "user", "Dee")
)

func newArticleTestService() (*articleService, *stubArticles) {
	repo := newStubArticles()
	n := 0
	svc := &articleService{
		repo: repo,
		now:  func() time.Time { return deskNow },
		newSuffix: func() (string, error) {
			n++
			return "abc23" + string(suffixAlphabet[n%len(suffixAlphabet)]), nil
		},
	}
	return svc, repo
}

// doc builds a body from paragraphs of text.
func doc(paragraphs ...string) json.RawMessage {
	var content []map[string]any
	for _, p := range paragraphs {
		content = append(content, map[string]any{
			"type":    "paragraph",
			"content": []map[string]any{{"type": "text", "text": p}},
		})
	}
	raw, _ := json.Marshal(map[string]any{"type": "doc", "content": content})
	return raw
}

func draftInput(title string) ArticleInput {
	return ArticleInput{Title: title, Body: doc("Arsenal pressed high from the first minute.")}
}

func kindOf(t *testing.T, err error) apperr.Kind {
	t.Helper()
	require.Error(t, err)
	return apperr.KindOf(err)
}

// --- the workflow ----------------------------------------------------------

func TestArticleEditorWorkflowNeedsAnAdministrator(t *testing.T) {
	svc, _ := newArticleTestService()

	a, err := svc.Create(editor, draftInput("How Arsenal press"))
	require.NoError(t, err)
	assert.Equal(t, domain.ArticleDraft, a.Status)
	assert.Equal(t, "Ana", a.AuthorName, "the byline comes from the token")

	a, err = svc.Submit(editor, a.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.ArticleInReview, a.Status)
	assert.NotNil(t, a.SubmittedAt)

	_, err = svc.GetLive(context.Background(), a.Slug)
	assert.Equal(t, apperr.KindNotFound, kindOf(t, err), "nothing is public until an admin publishes it")

	_, err = svc.Update(editor, a.ID, draftInput("Changed my mind"))
	assert.Equal(t, apperr.KindForbidden, kindOf(t, err), "locked while in review")

	a, err = svc.Submit(admin, a.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.ArticlePublished, a.Status)
	assert.Equal(t, "Chief", a.ReviewerName)

	live, err := svc.GetLive(context.Background(), a.Slug)
	require.NoError(t, err)
	assert.Equal(t, "How Arsenal press", live.Title)
	assert.Empty(t, live.ReviewerName, "review details are not for readers")

	_, err = svc.Update(editor, a.ID, draftInput("Typo fix"))
	assert.Equal(t, apperr.KindForbidden, kindOf(t, err), "an editor cannot change a published article")
	_, err = svc.Update(admin, a.ID, draftInput("How Arsenal press, revisited"))
	require.NoError(t, err, "an admin can")
}

func TestAnAdministratorsOwnArticlePublishesWithoutReview(t *testing.T) {
	svc, _ := newArticleTestService()
	a, err := svc.Create(admin, draftInput("Inverted full-backs"))
	require.NoError(t, err)

	a, err = svc.Submit(admin, a.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.ArticlePublished, a.Status)
	assert.True(t, a.Live(deskNow))
	assert.Equal(t, deskNow, *a.FirstPublishedAt)
}

func TestRequestChangesSendsAnArticleBackWithANote(t *testing.T) {
	svc, _ := newArticleTestService()
	a, _ := svc.Create(editor, draftInput("Low blocks"))
	_, err := svc.RequestChanges(admin, a.ID, "too early")
	assert.Equal(t, apperr.KindConflict, kindOf(t, err), "only from review")

	_, _ = svc.Submit(editor, a.ID, nil)
	_, err = svc.RequestChanges(other, a.ID, "nope")
	assert.Equal(t, apperr.KindForbidden, kindOf(t, err), "editors do not review")
	_, err = svc.RequestChanges(admin, a.ID, "   ")
	assert.Equal(t, apperr.KindInvalid, kindOf(t, err), "a note is required")

	a, err = svc.RequestChanges(admin, a.ID, "Add the second-half numbers.")
	require.NoError(t, err)
	assert.Equal(t, domain.ArticleChangesRequested, a.Status)
	assert.Equal(t, "Add the second-half numbers.", a.ReviewNote)

	_, err = svc.Update(editor, a.ID, draftInput("Low blocks, with numbers"))
	require.NoError(t, err, "the writer can edit again")
	a, err = svc.Submit(editor, a.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.ArticleInReview, a.Status)
}

func TestSchedulingGoesLiveByTheClock(t *testing.T) {
	svc, _ := newArticleTestService()
	a, _ := svc.Create(editor, draftInput("Derby preview"))
	at := deskNow.Add(48 * time.Hour)

	a, err := svc.Submit(editor, a.ID, &at)
	require.NoError(t, err)
	assert.Equal(t, at, *a.PublishAt, "the writer's requested time")

	// Approving without a time keeps the one asked for.
	a, err = svc.Submit(admin, a.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, domain.ArticlePublished, a.Status)
	assert.Equal(t, at, *a.PublishAt)

	_, err = svc.GetLive(context.Background(), a.Slug)
	assert.Equal(t, apperr.KindNotFound, kindOf(t, err), "scheduled, not yet live")
	items, _ := svc.ListLive(context.Background(), nil, domain.NewPage(10, 0))
	assert.Empty(t, items)

	svc.now = func() time.Time { return at.Add(time.Minute) }
	_, err = svc.GetLive(context.Background(), a.Slug)
	require.NoError(t, err, "live once the time passes, with nothing run at that moment")

	_, err = svc.Submit(admin, a.ID, ptr(at.Add(time.Hour*24)))
	assert.Equal(t, apperr.KindConflict, kindOf(t, err), "a live article is not rescheduled in place")
}

func TestSchedulingBounds(t *testing.T) {
	svc, _ := newArticleTestService()
	a, _ := svc.Create(admin, draftInput("Far ahead"))

	_, err := svc.Submit(admin, a.ID, ptr(deskNow.Add(400*24*time.Hour)))
	assert.Equal(t, apperr.KindInvalid, kindOf(t, err))

	a, err = svc.Submit(admin, a.ID, ptr(deskNow.Add(-time.Hour)))
	require.NoError(t, err)
	assert.Equal(t, deskNow, *a.PublishAt, "a time already past means now")
}

func TestWithdrawReturnsAnArticleToDraft(t *testing.T) {
	svc, _ := newArticleTestService()
	a, _ := svc.Create(editor, draftInput("Set pieces"))
	_, err := svc.Withdraw(editor, a.ID)
	assert.Equal(t, apperr.KindConflict, kindOf(t, err), "already a draft")

	_, _ = svc.Submit(editor, a.ID, nil)
	a, err = svc.Withdraw(editor, a.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ArticleDraft, a.Status)

	_, _ = svc.Submit(editor, a.ID, nil)
	a, _ = svc.Submit(admin, a.ID, nil)
	slug := a.Slug
	err = svc.Delete(editor, a.ID)
	assert.Equal(t, apperr.KindForbidden, kindOf(t, err), "a published article is withdrawn before it is deleted")

	a, err = svc.Withdraw(editor, a.ID)
	require.NoError(t, err, "a writer can take their own article down")
	assert.Nil(t, a.PublishAt)
	_, err = svc.GetLive(context.Background(), slug)
	assert.Equal(t, apperr.KindNotFound, kindOf(t, err))
	require.NoError(t, svc.Delete(editor, a.ID))
}

func TestTheDeskShowsEditorsOnlyTheirOwn(t *testing.T) {
	svc, _ := newArticleTestService()
	mine, _ := svc.Create(editor, draftInput("Mine"))
	theirs, _ := svc.Create(other, draftInput("Theirs"))
	_, _ = svc.Create(admin, draftInput("The admin's"))
	_, _ = svc.Submit(other, theirs.ID, nil)

	list, err := svc.DeskList(editor, ArticleDeskQuery{}, domain.NewPage(50, 0))
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, mine.ID, list[0].ID)

	_, err = svc.DeskGet(editor, theirs.ID)
	assert.Equal(t, apperr.KindNotFound, kindOf(t, err), "someone else's article does not exist, as far as an editor can tell")

	all, err := svc.DeskList(admin, ArticleDeskQuery{}, domain.NewPage(50, 0))
	require.NoError(t, err)
	assert.Len(t, all, 3)
	queue, err := svc.DeskList(admin, ArticleDeskQuery{Status: "in_review"}, domain.NewPage(50, 0))
	require.NoError(t, err)
	require.Len(t, queue, 1)
	assert.Equal(t, theirs.ID, queue[0].ID)
	own, err := svc.DeskList(admin, ArticleDeskQuery{Mine: true}, domain.NewPage(50, 0))
	require.NoError(t, err)
	assert.Len(t, own, 1)

	_, err = svc.DeskList(admin, ArticleDeskQuery{Status: "lost"}, domain.NewPage(50, 0))
	assert.Equal(t, apperr.KindInvalid, kindOf(t, err))
}

func TestOnlyEditorsAndAdministratorsWrite(t *testing.T) {
	svc, _ := newArticleTestService()
	_, err := svc.Create(member, draftInput("A fan's view"))
	assert.Equal(t, apperr.KindForbidden, kindOf(t, err))
	_, err = svc.DeskList(member, ArticleDeskQuery{}, domain.NewPage(10, 0))
	assert.Equal(t, apperr.KindForbidden, kindOf(t, err))
	_, err = svc.Create(context.Background(), draftInput("Nobody"))
	assert.Equal(t, apperr.KindUnauthorized, kindOf(t, err))
}

func TestSubmittingNeedsATitleAndText(t *testing.T) {
	svc, _ := newArticleTestService()
	a, _ := svc.Create(editor, ArticleInput{Body: doc("Some words.")})
	_, err := svc.Submit(editor, a.ID, nil)
	assert.Equal(t, apperr.KindInvalid, kindOf(t, err), "no title")

	b, _ := svc.Create(editor, ArticleInput{Title: "Empty"})
	_, err = svc.Submit(editor, b.ID, nil)
	assert.Equal(t, apperr.KindInvalid, kindOf(t, err), "no text")
}

func TestTheAddressFollowsTheTitleUntilFirstPublished(t *testing.T) {
	svc, _ := newArticleTestService()
	a, err := svc.Create(admin, ArticleInput{})
	require.NoError(t, err)
	suffix := slugSuffix(a.Slug)
	assert.Equal(t, "article-"+suffix, a.Slug)

	a, err = svc.Update(admin, a.ID, draftInput("Atlético's low block, explained"))
	require.NoError(t, err)
	assert.Equal(t, "atleticos-low-block-explained-"+suffix, a.Slug)

	a, _ = svc.Submit(admin, a.ID, nil)
	a, _ = svc.Withdraw(admin, a.ID)
	a, err = svc.Update(admin, a.ID, draftInput("A new headline"))
	require.NoError(t, err)
	assert.Equal(t, "atleticos-low-block-explained-"+suffix, a.Slug, "fixed once it has been public")
}

func TestASlugCollisionDrawsAnotherSuffix(t *testing.T) {
	svc, repo := newArticleTestService()
	calls := 0
	svc.newSuffix = func() (string, error) {
		calls++
		if calls < 3 {
			return "aaaaaa", nil
		}
		return "bbbbbb", nil
	}
	_, err := svc.Create(editor, draftInput("Same title"))
	require.NoError(t, err)
	b, err := svc.Create(editor, draftInput("Same title"))
	require.NoError(t, err)
	assert.Equal(t, "same-title-bbbbbb", b.Slug)
	assert.Len(t, repo.articles, 2)
}

func TestTagsAreNormalised(t *testing.T) {
	svc, _ := newArticleTestService()
	in := draftInput("Tagged")
	in.Tags = []string{" Pressing ", "#pressing", "Premier  League", ""}
	a, err := svc.Create(admin, in)
	require.NoError(t, err)
	assert.Equal(t, domain.StringList{"pressing", "premier league"}, a.Tags)
	_, _ = svc.Submit(admin, a.ID, nil)

	tag := "Pressing"
	items, err := svc.ListLive(context.Background(), &tag, domain.NewPage(10, 0))
	require.NoError(t, err)
	assert.Len(t, items, 1)

	in.Tags = strings.Split("a,b,c,d,e,f,g,h,i", ",")
	_, err = svc.Create(admin, in)
	assert.Equal(t, apperr.KindInvalid, kindOf(t, err))
}

func TestDeletingAnAccountKeepsItsPublishedArticles(t *testing.T) {
	svc, repo := newArticleTestService()
	draft, _ := svc.Create(editor, draftInput("Unfinished"))
	pub, _ := svc.Create(editor, draftInput("Finished"))
	_, _ = svc.Submit(editor, pub.ID, nil)
	_, _ = svc.Submit(admin, pub.ID, nil)

	n, err := repo.DeleteUnpublishedByAuthor(context.Background(), writerID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	_, err = repo.GetByID(context.Background(), draft.ID)
	assert.Error(t, err)
	_, err = repo.GetByID(context.Background(), pub.ID)
	assert.NoError(t, err)
}

// --- the body --------------------------------------------------------------

func TestTheBodyIsChecked(t *testing.T) {
	node := func(v map[string]any) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{v}})
		return raw
	}
	link := func(href string) json.RawMessage {
		return node(map[string]any{"type": "paragraph", "content": []any{map[string]any{
			"type": "text", "text": "here",
			"marks": []any{map[string]any{"type": "link", "attrs": map[string]any{"href": href}}},
		}}})
	}

	ok := []json.RawMessage{
		link("https://www.uefa.com/match/1"),
		link("/players/123"),
		link("mailto:desk@example.com"),
		node(map[string]any{"type": "figure", "attrs": map[string]any{"src": "https://img.example.com/a.jpg"}}),
		node(map[string]any{"type": "tacticBoard", "attrs": map[string]any{"slug": "aB3dE5fG7hJ9"}}),
		node(map[string]any{"type": "video", "attrs": map[string]any{"provider": "youtube", "id": "dQw4w9WgXcQ"}}),
		node(map[string]any{"type": "heading", "attrs": map[string]any{"level": 2}}),
	}
	for _, body := range ok {
		_, err := checkArticleBody(body)
		assert.NoError(t, err, string(body))
	}

	bad := []json.RawMessage{
		link("javascript:alert(1)"),
		link("//evil.example.com"),
		link("data:text/html,hi"),
		node(map[string]any{"type": "figure", "attrs": map[string]any{"src": "http://img.example.com/a.jpg"}}),
		node(map[string]any{"type": "figure", "attrs": map[string]any{"src": "javascript:alert(1)"}}),
		node(map[string]any{"type": "tacticBoard", "attrs": map[string]any{"slug": "../../x"}}),
		node(map[string]any{"type": "video", "attrs": map[string]any{"provider": "evil", "id": "x"}}),
		node(map[string]any{"type": "heading", "attrs": map[string]any{"level": 1}}),
		node(map[string]any{"type": "iframe"}),
		json.RawMessage(`{"type":"paragraph"}`),
		json.RawMessage(`[]`),
	}
	for _, body := range bad {
		_, err := checkArticleBody(body)
		assert.Equal(t, apperr.KindInvalid, kindOf(t, err), string(body))
	}
}

func TestExcerptAndReadingTime(t *testing.T) {
	long := strings.Repeat("word ", 700)
	facts, err := checkArticleBody(doc("First paragraph.", long))
	require.NoError(t, err)
	assert.Equal(t, 4, facts.readingMinutes, "702 words at 230 a minute")
	assert.True(t, strings.HasPrefix(facts.excerpt, "First paragraph. word word"))
	assert.True(t, strings.HasSuffix(facts.excerpt, "…"))
	assert.LessOrEqual(t, len([]rune(facts.excerpt)), articleExcerptAt+1)

	facts, _ = checkArticleBody(doc("Short."))
	assert.Equal(t, "Short.", facts.excerpt)
	assert.Equal(t, 1, facts.readingMinutes)
}

func TestSlugWords(t *testing.T) {
	cases := map[string]string{
		"How Arsenal press":                   "how-arsenal-press",
		"Atlético's low block, explained":     "atleticos-low-block-explained",
		"  3-4-3 vs. 4-4-2!  ":                "3-4-3-vs-4-4-2",
		"Ødegaard & Saka: a right side":       "ødegaard-saka-a-right-side",
		"":                                    "",
		"!!!":                                 "",
		strings.Repeat("Counterpressing ", 8): "counterpressing-counterpressing-counterpressing",
	}
	for in, want := range cases {
		assert.Equal(t, want, slugWords(in), in)
	}
}
