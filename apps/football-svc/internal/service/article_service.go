package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
)

// Bounds on an article.
const (
	MaxArticleTitleLength    = 160
	MaxArticleSubtitleLength = 300
	MaxArticleCaptionLength  = 300
	MaxArticleBodyBytes      = 512 << 10
	MaxArticleTags           = 8
	MaxArticleTagLength      = 32
	MaxReviewNoteLength      = 2000

	// MaxArticlesPerAuthor bounds one account's desk. Far above what anyone
	// writes; it is there so a compromised editor account cannot fill the
	// table.
	MaxArticlesPerAuthor = 1000

	// MaxScheduleAhead is how far ahead an article may be scheduled. A date
	// further out is far more likely a typo in the year than a plan.
	MaxScheduleAhead = 366 * 24 * time.Hour
)

// suffixAlphabet is lower case only: the suffix sits in a readable address,
// where mixed case looks like noise. Still no 0/o or 1/l.
const suffixAlphabet = "23456789abcdefghijkmnpqrstuvwxyz"

const suffixLength = 6

// ArticleService is the analysis section and the editorial desk behind it.
//
// Who may do what:
//
//   - Anyone reads a live article -- published, and its publish time passed.
//   - Editors and administrators write. A plain member has no desk.
//   - An editor sees and edits only their own articles, and only while they
//     are drafts or have been sent back. Submitting hands one to the
//     administrators; it is locked until they publish it or send it back, or
//     until its writer withdraws it.
//   - An administrator sees every article, edits any of them in any state,
//     publishes or schedules them, and sends them back with a note. Their own
//     articles need nobody's approval: submitting one publishes it.
type ArticleService interface {
	GetLive(ctx context.Context, slug string) (*domain.Article, error)
	ListLive(ctx context.Context, tag *string, page domain.Page) ([]domain.ArticleSummary, error)

	DeskList(ctx context.Context, query ArticleDeskQuery, page domain.Page) ([]domain.ArticleSummary, error)
	DeskGet(ctx context.Context, id string) (*domain.Article, error)
	Create(ctx context.Context, input ArticleInput) (*domain.Article, error)
	Update(ctx context.Context, id string, input ArticleInput) (*domain.Article, error)
	// Submit sends an editor's article for review, or publishes an
	// administrator's -- or, from an administrator, approves someone else's.
	// publishAt schedules it; nil or a time already past means now.
	Submit(ctx context.Context, id string, publishAt *time.Time) (*domain.Article, error)
	// RequestChanges sends an article in review back to its writer.
	RequestChanges(ctx context.Context, id string, note string) (*domain.Article, error)
	// Withdraw returns an article in review, or a published one, to draft.
	Withdraw(ctx context.Context, id string) (*domain.Article, error)
	Delete(ctx context.Context, id string) error
}

// ArticleInput is the writable surface of an article. Who wrote it, its
// status and its address are the service's, never the request's.
type ArticleInput struct {
	Title        string
	Subtitle     string
	CoverURL     string
	CoverCaption string
	Body         json.RawMessage
	Tags         []string
}

// ArticleDeskQuery narrows the desk's list.
type ArticleDeskQuery struct {
	Status string
	// Mine narrows an administrator's list to their own articles. An editor's
	// list is always only their own.
	Mine bool
}

type articleService struct {
	repo repository.ArticleRepository
	// Swappable for tests: the clock, and the suffix so a collision can be
	// forced.
	now       func() time.Time
	newSuffix func() (string, error)
}

func NewArticleService(repo repository.ArticleRepository) ArticleService {
	return &articleService{repo: repo, now: time.Now, newSuffix: randomSuffix}
}

// --- reading ---------------------------------------------------------------

func (s *articleService) GetLive(ctx context.Context, slug string) (*domain.Article, error) {
	if slug == "" || len(slug) > 200 {
		return nil, apperr.NotFound("article not found")
	}
	a, err := s.repo.GetLive(ctx, slug, s.now())
	if err != nil {
		return nil, err
	}
	return a.ForReaders(), nil
}

func (s *articleService) ListLive(ctx context.Context, tag *string, page domain.Page) ([]domain.ArticleSummary, error) {
	if tag != nil {
		t := normaliseTag(*tag)
		tag = &t
	}
	items, err := s.repo.ListLive(ctx, tag, s.now(), page)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].ReviewNote = ""
		items[i].SubmittedAt = nil
	}
	return items, nil
}

// --- the desk --------------------------------------------------------------

func (s *articleService) DeskList(ctx context.Context, query ArticleDeskQuery, page domain.Page) ([]domain.ArticleSummary, error) {
	claims, err := s.writer(ctx)
	if err != nil {
		return nil, err
	}

	var filter domain.ArticleDeskFilter
	if query.Status != "" {
		status := domain.ArticleStatus(query.Status)
		if !status.Valid() {
			return nil, apperr.Invalid("status must be draft, in_review, changes_requested or published")
		}
		filter.Status = &status
	}
	if claims.Role != RoleAdmin || query.Mine {
		filter.AuthorID = &claims.UserID
	}
	return s.repo.ListDesk(ctx, filter, page)
}

func (s *articleService) DeskGet(ctx context.Context, id string) (*domain.Article, error) {
	claims, err := s.writer(ctx)
	if err != nil {
		return nil, err
	}
	return s.reachable(ctx, claims, id)
}

func (s *articleService) Create(ctx context.Context, input ArticleInput) (*domain.Article, error) {
	claims, err := s.writer(ctx)
	if err != nil {
		return nil, err
	}

	a := &domain.Article{
		AuthorID:   claims.UserID,
		AuthorName: claims.Username,
		Status:     domain.ArticleDraft,
	}
	if err := applyArticleInput(a, input); err != nil {
		return nil, err
	}

	count, err := s.repo.CountByAuthor(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if count >= MaxArticlesPerAuthor {
		return nil, apperr.Invalid("you have reached the limit of 1000 articles; delete some drafts to make room")
	}

	// The suffix is drawn and the insert retried on the rare collision, rather
	// than checked first: only the unique constraint can settle a race.
	const attempts = 3
	for i := 0; i < attempts; i++ {
		suffix, err := s.newSuffix()
		if err != nil {
			return nil, apperr.Internal(err)
		}
		a.Slug = articleSlug(a.Title, suffix)
		err = s.repo.Create(ctx, a)
		if err == nil {
			return a, nil
		}
		if apperr.KindOf(err) != apperr.KindConflict {
			return nil, err
		}
	}
	return nil, apperr.Conflict("could not find a free address for this article; try again")
}

func (s *articleService) Update(ctx context.Context, id string, input ArticleInput) (*domain.Article, error) {
	claims, err := s.writer(ctx)
	if err != nil {
		return nil, err
	}
	a, err := s.reachable(ctx, claims, id)
	if err != nil {
		return nil, err
	}

	if claims.Role != RoleAdmin {
		switch a.Status {
		case domain.ArticleInReview:
			return nil, apperr.Forbidden("this article is waiting for review; withdraw it to make changes")
		case domain.ArticlePublished:
			return nil, apperr.Forbidden("only an administrator can change a published article; withdraw it to edit it again")
		}
	}

	if err := applyArticleInput(a, input); err != nil {
		return nil, err
	}
	// The address follows the title until the article is first published,
	// and never moves after: a shared link has to keep working.
	if a.FirstPublishedAt == nil {
		a.Slug = articleSlug(a.Title, slugSuffix(a.Slug))
	}

	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *articleService) Submit(ctx context.Context, id string, publishAt *time.Time) (*domain.Article, error) {
	claims, err := s.writer(ctx)
	if err != nil {
		return nil, err
	}
	a, err := s.reachable(ctx, claims, id)
	if err != nil {
		return nil, err
	}

	if err := readyToSubmit(a); err != nil {
		return nil, err
	}

	now := s.now().UTC()
	if publishAt != nil {
		t := publishAt.UTC().Truncate(time.Second)
		if t.After(now.Add(MaxScheduleAhead)) {
			return nil, apperr.Invalid("an article can be scheduled at most a year ahead")
		}
		if !t.After(now) {
			publishAt = nil
		} else {
			publishAt = &t
		}
	}

	if claims.Role == RoleAdmin {
		return s.publish(ctx, claims, a, publishAt, now)
	}

	switch a.Status {
	case domain.ArticleDraft, domain.ArticleChangesRequested:
	case domain.ArticleInReview:
		return nil, apperr.Conflict("this article is already waiting for review")
	default:
		return nil, apperr.Conflict("this article is already published")
	}
	a.Status = domain.ArticleInReview
	a.SubmittedAt = &now
	// For an article in review, publish_at is the writer's request, which
	// the reviewer can keep or change.
	a.PublishAt = publishAt

	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// publish is an administrator publishing, scheduling or rescheduling.
func (s *articleService) publish(ctx context.Context, claims *auth.Claims, a *domain.Article, publishAt *time.Time, now time.Time) (*domain.Article, error) {
	if a.Live(now) {
		return nil, apperr.Conflict("this article is already live; withdraw it first to schedule it again")
	}

	// Approving without naming a time keeps the time the writer asked for,
	// if it is still ahead.
	if publishAt == nil && a.Status == domain.ArticleInReview && a.PublishAt != nil && a.PublishAt.After(now) {
		publishAt = a.PublishAt
	}
	at := now
	if publishAt != nil {
		at = *publishAt
	}

	a.Status = domain.ArticlePublished
	a.PublishAt = &at
	if a.FirstPublishedAt == nil {
		a.FirstPublishedAt = &at
	}
	a.ReviewerID = &claims.UserID
	a.ReviewerName = claims.Username
	a.ReviewedAt = &now
	a.ReviewNote = ""

	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *articleService) RequestChanges(ctx context.Context, id string, note string) (*domain.Article, error) {
	claims, err := s.writer(ctx)
	if err != nil {
		return nil, err
	}
	if claims.Role != RoleAdmin {
		return nil, apperr.Forbidden("only an administrator reviews articles")
	}
	a, err := s.reachable(ctx, claims, id)
	if err != nil {
		return nil, err
	}
	if a.Status != domain.ArticleInReview {
		return nil, apperr.Conflict("only an article waiting for review can be sent back")
	}

	note = strings.TrimSpace(note)
	if note == "" {
		return nil, apperr.Invalid("say what needs to change, so the writer knows what to do")
	}
	if utf8.RuneCountInString(note) > MaxReviewNoteLength {
		return nil, apperr.Invalid("a review note can be at most 2000 characters")
	}

	now := s.now().UTC()
	a.Status = domain.ArticleChangesRequested
	a.ReviewerID = &claims.UserID
	a.ReviewerName = claims.Username
	a.ReviewedAt = &now
	a.ReviewNote = note

	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *articleService) Withdraw(ctx context.Context, id string) (*domain.Article, error) {
	claims, err := s.writer(ctx)
	if err != nil {
		return nil, err
	}
	a, err := s.reachable(ctx, claims, id)
	if err != nil {
		return nil, err
	}

	switch a.Status {
	case domain.ArticleInReview:
		// The writer's requested time goes with the request.
		a.PublishAt = nil
		a.SubmittedAt = nil
	case domain.ArticlePublished:
		a.PublishAt = nil
	default:
		return nil, apperr.Conflict("this article is not in review or published")
	}
	a.Status = domain.ArticleDraft

	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Delete removes an article. Its writer may delete one that is not
// published; a published one is an administrator's to take down, so that an
// article readers have linked to does not vanish without one of them knowing.
func (s *articleService) Delete(ctx context.Context, id string) error {
	claims, err := s.writer(ctx)
	if err != nil {
		return err
	}
	a, err := s.reachable(ctx, claims, id)
	if err != nil {
		return err
	}
	if claims.Role != RoleAdmin && a.Status == domain.ArticlePublished {
		return apperr.Forbidden("withdraw a published article before deleting it")
	}
	return s.repo.Delete(ctx, id)
}

// --- rules -----------------------------------------------------------------

// writer requires an editor or administrator.
func (s *articleService) writer(ctx context.Context) (*auth.Claims, error) {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if claims.Role != RoleAdmin && claims.Role != RoleEditor {
		return nil, apperr.Forbidden("only editors and administrators write articles")
	}
	return claims, nil
}

// reachable loads an article the caller may see on the desk: their own, or
// any for an administrator. Someone else's answers not found rather than
// forbidden, so an editor cannot learn which ids exist.
func (s *articleService) reachable(ctx context.Context, claims *auth.Claims, id string) (*domain.Article, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if claims.Role != RoleAdmin && a.AuthorID != claims.UserID {
		return nil, apperr.NotFound("article not found")
	}
	return a, nil
}

// readyToSubmit is what an article needs before anyone else reads it.
func readyToSubmit(a *domain.Article) error {
	if strings.TrimSpace(a.Title) == "" {
		return apperr.Invalid("give the article a title before submitting it")
	}
	facts, err := checkArticleBody(a.Body)
	if err != nil {
		return err
	}
	if facts.words == 0 {
		return apperr.Invalid("the article has no text yet")
	}
	return nil
}

// applyArticleInput validates the input and writes it, with the facts
// derived from the body, onto a.
func applyArticleInput(a *domain.Article, input ArticleInput) error {
	title := strings.Join(strings.Fields(input.Title), " ")
	if utf8.RuneCountInString(title) > MaxArticleTitleLength {
		return apperr.Invalid("a title can be at most 160 characters")
	}
	subtitle := strings.TrimSpace(input.Subtitle)
	if utf8.RuneCountInString(subtitle) > MaxArticleSubtitleLength {
		return apperr.Invalid("a subtitle can be at most 300 characters")
	}

	cover := strings.TrimSpace(input.CoverURL)
	if cover != "" && !SafeImageURL(cover) {
		return apperr.Invalid("the cover image must be an https:// address")
	}
	caption := strings.TrimSpace(input.CoverCaption)
	if utf8.RuneCountInString(caption) > MaxArticleCaptionLength {
		return apperr.Invalid("a cover caption can be at most 300 characters")
	}

	body := bytes.TrimSpace(input.Body)
	if len(body) == 0 {
		body = []byte(`{"type":"doc","content":[]}`)
	}
	if len(body) > MaxArticleBodyBytes {
		return apperr.Invalid("the article is too long to save")
	}
	facts, err := checkArticleBody(body)
	if err != nil {
		return err
	}

	tags, err := normaliseTags(input.Tags)
	if err != nil {
		return err
	}

	a.Title = title
	a.Subtitle = subtitle
	a.CoverURL = cover
	a.CoverCaption = caption
	a.Body = json.RawMessage(body)
	a.Excerpt = facts.excerpt
	a.ReadingMinutes = facts.readingMinutes
	a.Tags = tags
	return nil
}

// normaliseTags lower-cases and de-duplicates tags, so "Pressing" and
// "pressing " are one topic and ?tag= finds both.
func normaliseTags(in []string) (domain.StringList, error) {
	out := domain.StringList{}
	seen := map[string]bool{}
	for _, raw := range in {
		t := normaliseTag(raw)
		if t == "" || seen[t] {
			continue
		}
		if utf8.RuneCountInString(t) > MaxArticleTagLength {
			return nil, apperr.Invalid("a tag can be at most 32 characters")
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > MaxArticleTags {
		return nil, apperr.Invalid("an article can have at most 8 tags")
	}
	return out, nil
}

func normaliseTag(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimSpace(raw), "#")), " "))
}

func articleSlug(title, suffix string) string {
	if words := slugWords(title); words != "" {
		return words + "-" + suffix
	}
	return "article-" + suffix
}

// slugSuffix recovers the random half of an address, which stays the same
// when the title changes.
func slugSuffix(slug string) string {
	if i := strings.LastIndexByte(slug, '-'); i >= 0 && len(slug)-i-1 == suffixLength {
		return slug[i+1:]
	}
	// Not one of ours; keep it whole rather than guess.
	return slug
}

func randomSuffix() (string, error) {
	max := big.NewInt(int64(len(suffixAlphabet)))
	out := make([]byte, suffixLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = suffixAlphabet[n.Int64()]
	}
	return string(out), nil
}
