package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// articlesAgainstPostgres proves what the in-memory stub cannot: the JSONB and
// TEXT[] round trips, the live filter on status and publish time, the tag
// containment query, the desk filters, and the constraints.
func articlesAgainstPostgres(t *testing.T, ctx context.Context, db *sqlx.DB) {
	articles := repository.NewPostgresArticleRepository(db)
	const ana = "11111111-1111-1111-1111-111111111111"
	const ben = "22222222-2222-2222-2222-222222222222"
	now := time.Now().UTC().Truncate(time.Second)

	mk := func(slug, author, title string, tags ...string) *domain.Article {
		a := &domain.Article{
			Slug: slug, AuthorID: author, AuthorName: "writer", Title: title,
			Body:   json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hi"}]}]}`),
			Tags:   tags,
			Status: domain.ArticleDraft, ReadingMinutes: 1,
		}
		require.NoError(t, articles.Create(ctx, a))
		require.NotEmpty(t, a.ID)
		return a
	}
	publish := func(a *domain.Article, at time.Time) {
		a.Status = domain.ArticlePublished
		a.PublishAt = &at
		a.FirstPublishedAt = &at
		require.NoError(t, articles.Update(ctx, a))
	}

	old := mk("pressing-traps-aaaaaa", ana, "Pressing traps", "pressing", "premier league")
	recent := mk("low-blocks-bbbbbb", ana, "Low blocks", "defending")
	scheduled := mk("derby-preview-cccccc", ben, "Derby preview", "pressing")
	draft := mk("unfinished-dddddd", ben, "Unfinished", "pressing")
	publish(old, now.Add(-48*time.Hour))
	publish(recent, now.Add(-time.Hour))
	publish(scheduled, now.Add(24*time.Hour))

	err := articles.Create(ctx, &domain.Article{
		Slug: "low-blocks-bbbbbb", AuthorID: ana, Body: json.RawMessage(`{}`), Status: domain.ArticleDraft,
	})
	assert.Error(t, err, "slugs are unique")

	live, err := articles.ListLive(ctx, nil, now, domain.NewPage(10, 0))
	require.NoError(t, err)
	require.Len(t, live, 2, "neither the draft nor the scheduled article is live")
	assert.Equal(t, "Low blocks", live[0].Title, "newest first")
	assert.Equal(t, domain.StringList{"pressing", "premier league"}, live[1].Tags)

	tag := "pressing"
	tagged, err := articles.ListLive(ctx, &tag, now, domain.NewPage(10, 0))
	require.NoError(t, err)
	require.Len(t, tagged, 1)
	assert.Equal(t, "Pressing traps", tagged[0].Title)

	later, err := articles.ListLive(ctx, &tag, now.Add(25*time.Hour), domain.NewPage(10, 0))
	require.NoError(t, err)
	assert.Len(t, later, 2, "the scheduled one appears once its time comes")

	got, err := articles.GetLive(ctx, "pressing-traps-aaaaaa", now)
	require.NoError(t, err)
	assert.JSONEq(t, string(old.Body), string(got.Body))
	_, err = articles.GetLive(ctx, "derby-preview-cccccc", now)
	assert.Error(t, err, "scheduled is not live")
	_, err = articles.GetLive(ctx, "unfinished-dddddd", now)
	assert.Error(t, err, "a draft is not live")

	author := ben
	status := domain.ArticleDraft
	desk, err := articles.ListDesk(ctx, domain.ArticleDeskFilter{AuthorID: &author}, domain.NewPage(10, 0))
	require.NoError(t, err)
	assert.Len(t, desk, 2)
	drafts, err := articles.ListDesk(ctx, domain.ArticleDeskFilter{Status: &status}, domain.NewPage(10, 0))
	require.NoError(t, err)
	require.Len(t, drafts, 1)
	assert.Equal(t, draft.ID, drafts[0].ID)

	// Review details round-trip.
	reviewer := ana
	reviewed := now
	draft.Status = domain.ArticleChangesRequested
	draft.ReviewerID = &reviewer
	draft.ReviewerName = "Chief"
	draft.ReviewedAt = &reviewed
	draft.ReviewNote = "More numbers"
	require.NoError(t, articles.Update(ctx, draft))
	reread, err := articles.GetByID(ctx, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, "More numbers", reread.ReviewNote)
	assert.Equal(t, reviewer, *reread.ReviewerID)

	// A published row without a time breaks a constraint.
	broken := *reread
	broken.Status = domain.ArticlePublished
	broken.PublishAt = nil
	assert.Error(t, articles.Update(ctx, &broken))

	count, err := articles.CountByAuthor(ctx, ben)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	removed, err := articles.DeleteUnpublishedByAuthor(ctx, ben)
	require.NoError(t, err)
	assert.EqualValues(t, 1, removed, "the scheduled, published one stays")

	require.NoError(t, articles.Delete(ctx, old.ID))
	assert.Error(t, articles.Delete(ctx, old.ID))
}
