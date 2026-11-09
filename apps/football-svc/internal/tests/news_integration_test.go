package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/news"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newsAgainstPostgres proves the SQL the in-memory store stands in for: the
// single running slot, the unique link, publishing, flags and the rolling
// 100-story window.
func newsAgainstPostgres(t *testing.T, ctx context.Context, db *sqlx.DB) {
	store := repository.NewNewsRepository(db)
	now := time.Now().UTC().Truncate(time.Second)

	id, ok, err := store.BeginRun(ctx, 20*time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	_, ok, err = store.BeginRun(ctx, 20*time.Minute)
	require.NoError(t, err)
	assert.False(t, ok, "only one run at a time")
	require.NoError(t, store.FinishRun(ctx, news.RunReport{ID: id, Status: "ok", Errors: []string{"bbc: timeout"}}))
	runs, err := store.LastRuns(ctx, 5)
	require.NoError(t, err)
	require.NotEmpty(t, runs)
	assert.Equal(t, []string{"bbc: timeout"}, runs[0].Errors)

	item := func(i int) news.Article {
		u := fmt.Sprintf("https://feed.example/%d", i)
		return news.Article{SourceKey: "bbc-sport", Title: fmt.Sprintf("Story %d", i), URL: u, Canonical: u, PublishedAt: now}
	}
	mk := func(i, score int) *news.Story {
		s := &news.Story{Headline: fmt.Sprintf("Story %d", i), URL: "https://feed.example", SourceName: "BBC Sport",
			Category: news.CategoryTransfers, Score: score, Scorer: "rules", SourceCount: 1, FirstSeenAt: now}
		require.NoError(t, store.CreateStory(ctx, s, []news.Article{item(i)}))
		return s
	}
	high, low := mk(1, 9), mk(2, 5)

	known, err := store.KnownCanonicals(ctx, []string{"https://feed.example/1", "https://feed.example/x"})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"https://feed.example/1": true}, known)

	require.NoError(t, store.AttachItems(ctx, high.ID, []news.Article{item(1), item(3)}, 2, nil),
		"an item already stored is skipped, not an error")

	n, err := store.PublishBest(ctx, now.Add(-time.Hour), news.PublishThreshold, 10, now)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the story scoring 7+ is published")

	top, err := store.Top(ctx, now.Add(-24*time.Hour), 5)
	require.NoError(t, err)
	require.Len(t, top, 1)
	assert.Equal(t, 2, top[0].SourceCount)

	pin := true
	pinned, err := store.SetFlags(ctx, low.ID, nil, &pin, now)
	require.NoError(t, err)
	assert.NotNil(t, pinned.PublishedAt, "pinning a candidate publishes it")
	feed, err := store.ListPublished(ctx, nil, 10, 0)
	require.NoError(t, err)
	require.Len(t, feed, 2)
	assert.Equal(t, low.ID, feed[0].ID, "pinned first")

	hide := true
	_, err = store.SetFlags(ctx, high.ID, &hide, nil, now)
	require.NoError(t, err)
	feed, err = store.ListPublished(ctx, nil, 10, 0)
	require.NoError(t, err)
	assert.Len(t, feed, 1, "hidden stories leave the feed")

	// The rolling window: 103 more published stories, then only 100 unpinned remain.
	for i := 10; i < 113; i++ {
		s := mk(i, 8)
		pub := now.Add(-time.Duration(i) * time.Minute)
		_, err := db.ExecContext(ctx, `UPDATE news_stories SET published_at = $2 WHERE id = $1`, s.ID, pub)
		require.NoError(t, err)
	}
	_, err = store.Prune(ctx, news.KeepPublished, now.Add(-news.CandidateTTL), now.Add(-news.RunLogTTL))
	require.NoError(t, err)
	var unpinned, items int
	require.NoError(t, db.GetContext(ctx, &unpinned, `SELECT COUNT(*) FROM news_stories WHERE published_at IS NOT NULL AND NOT pinned`))
	require.NoError(t, db.GetContext(ctx, &items, `SELECT COUNT(*) FROM news_items i JOIN news_stories s ON s.id = i.story_id`))
	assert.Equal(t, news.KeepPublished, unpinned)
	var orphans int
	require.NoError(t, db.GetContext(ctx, &orphans, `SELECT COUNT(*) FROM news_items WHERE story_id NOT IN (SELECT id FROM news_stories)`))
	assert.Zero(t, orphans, "a deleted story's items go with it")

	day := now.Truncate(24 * time.Hour)
	require.NoError(t, store.AddUsage(ctx, day, news.MeterNeurons, 120))
	require.NoError(t, store.AddUsage(ctx, day, news.MeterNeurons, -20))
	used, err := store.Usage(ctx, day, news.MeterNeurons)
	require.NoError(t, err)
	assert.Equal(t, 100, used)
}
