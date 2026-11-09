package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/scoutpulse/football-svc/internal/news"
)

// NewsRepository is the Postgres store behind the news pipeline. It
// implements news.Store.
type NewsRepository struct {
	db *sqlx.DB
}

func NewNewsRepository(db *sqlx.DB) *NewsRepository { return &NewsRepository{db: db} }

var _ news.Store = (*NewsRepository)(nil)

const storyColumns = `id, headline, url, source_name, attribution, summary, category, score,
	reason, scorer, source_count, scored_sources, first_seen_at, published_at, hidden, pinned,
	created_at, updated_at`

// --- usage -------------------------------------------------------------------

func (r *NewsRepository) Usage(ctx context.Context, day time.Time, meter string) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n,
		`SELECT COALESCE((SELECT amount FROM news_usage WHERE day = $1 AND meter = $2), 0)`,
		day.Format("2006-01-02"), meter)
	return n, err
}

func (r *NewsRepository) AddUsage(ctx context.Context, day time.Time, meter string, amount int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO news_usage (day, meter, amount) VALUES ($1, $2, GREATEST($3, 0))
		 ON CONFLICT (day, meter) DO UPDATE SET amount = GREATEST(news_usage.amount + $3, 0)`,
		day.Format("2006-01-02"), meter, amount)
	return err
}

// --- runs --------------------------------------------------------------------

func (r *NewsRepository) BeginRun(ctx context.Context, staleAfter time.Duration) (int64, bool, error) {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE news_runs SET status = 'failed', finished_at = NOW(),
		        errors = errors || '["abandoned: the run never finished"]'::jsonb
		  WHERE status = 'running' AND started_at < NOW() - make_interval(secs => $1)`,
		staleAfter.Seconds()); err != nil {
		return 0, false, err
	}
	var id int64
	err := r.db.GetContext(ctx, &id, `INSERT INTO news_runs (status) VALUES ('running') RETURNING id`)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == pgUniqueViolation {
			return 0, false, nil
		}
		return 0, false, err
	}
	return id, true, nil
}

func (r *NewsRepository) FinishRun(ctx context.Context, rep news.RunReport) error {
	errs := rep.Errors
	if errs == nil {
		errs = []string{}
	}
	payload, err := json.Marshal(errs)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE news_runs SET finished_at = NOW(), status = $2, fetched = $3, new_items = $4,
		        scored = $5, published = $6, pruned = $7, ai_neurons = $8, errors = $9
		  WHERE id = $1`,
		rep.ID, rep.Status, rep.Fetched, rep.NewItems, rep.Scored, rep.Published,
		rep.Pruned, rep.AINeurons, payload)
	return err
}

func (r *NewsRepository) LastRuns(ctx context.Context, n int) ([]news.RunReport, error) {
	rows, err := r.db.QueryxContext(ctx,
		`SELECT id, status, fetched, new_items, scored, published, pruned, ai_neurons, errors, started_at
		   FROM news_runs ORDER BY started_at DESC, id DESC LIMIT $1`, n)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []news.RunReport
	for rows.Next() {
		var rep news.RunReport
		var errs []byte
		if err := rows.Scan(&rep.ID, &rep.Status, &rep.Fetched, &rep.NewItems, &rep.Scored,
			&rep.Published, &rep.Pruned, &rep.AINeurons, &errs, &rep.StartedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(errs, &rep.Errors)
		if rep.Errors == nil {
			rep.Errors = []string{}
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

// --- ingest ------------------------------------------------------------------

func (r *NewsRepository) KnownCanonicals(ctx context.Context, canonicals []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(canonicals) == 0 {
		return out, nil
	}
	var found []string
	if err := r.db.SelectContext(ctx, &found,
		`SELECT canonical_url FROM news_items WHERE canonical_url = ANY($1)`,
		pq.Array(canonicals)); err != nil {
		return nil, err
	}
	for _, c := range found {
		out[c] = true
	}
	return out, nil
}

func (r *NewsRepository) RecentStories(ctx context.Context, since time.Time) ([]news.StoryRef, error) {
	rows, err := r.db.QueryxContext(ctx,
		`SELECT s.id, s.headline, s.published_at IS NOT NULL,
		        COALESCE(array_agg(i.title) FILTER (WHERE i.id IS NOT NULL), '{}'),
		        COALESCE(array_agg(DISTINCT i.source_key) FILTER (WHERE i.id IS NOT NULL), '{}')
		   FROM news_stories s
		   LEFT JOIN news_items i ON i.story_id = s.id
		  WHERE s.first_seen_at >= $1
		  GROUP BY s.id`, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []news.StoryRef
	for rows.Next() {
		var ref news.StoryRef
		if err := rows.Scan(&ref.ID, &ref.Headline, &ref.Published,
			pq.Array(&ref.Titles), pq.Array(&ref.SourceKeys)); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

func insertItems(ctx context.Context, tx *sqlx.Tx, storyID string, items []news.Article) error {
	for _, it := range items {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO news_items (story_id, source_key, title, url, canonical_url, published_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (canonical_url) DO NOTHING`,
			storyID, it.SourceKey, it.Title, it.URL, it.Canonical, it.PublishedAt); err != nil {
			return err
		}
	}
	return nil
}

func (r *NewsRepository) CreateStory(ctx context.Context, s *news.Story, items []news.Article) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.GetContext(ctx, &s.ID,
		`INSERT INTO news_stories (headline, url, source_name, attribution, summary, category,
		        score, reason, scorer, source_count, scored_sources, first_seen_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
		s.Headline, s.URL, s.SourceName, s.Attribution, s.Summary, s.Category, s.Score,
		s.Reason, s.Scorer, s.SourceCount, s.ScoredSources, s.FirstSeenAt); err != nil {
		return err
	}
	if err := insertItems(ctx, tx, s.ID, items); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *NewsRepository) AttachItems(ctx context.Context, storyID string, items []news.Article, outlets int, v *news.Verdict) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertItems(ctx, tx, storyID, items); err != nil {
		return err
	}
	if v != nil {
		_, err = tx.ExecContext(ctx,
			`UPDATE news_stories SET source_count = $2, scored_sources = $2, score = $3,
			        category = $4, reason = $5, scorer = $6,
			        summary = CASE WHEN $7 <> '' THEN $7 ELSE summary END,
			        updated_at = NOW()
			  WHERE id = $1`,
			storyID, outlets, v.Score, v.Category, v.Reason, v.Scorer, v.Summary)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE news_stories SET source_count = GREATEST(source_count, $2), updated_at = NOW()
			  WHERE id = $1`, storyID, outlets)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// --- publishing ----------------------------------------------------------------

func (r *NewsRepository) PublishedSince(ctx context.Context, t time.Time) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM news_stories WHERE published_at >= $1`, t)
	return n, err
}

func (r *NewsRepository) PublishBest(ctx context.Context, since time.Time, minScore, n int, now time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE news_stories SET published_at = $4, updated_at = NOW()
		  WHERE id IN (
		        SELECT id FROM news_stories
		         WHERE published_at IS NULL AND NOT hidden
		           AND score >= $2 AND first_seen_at >= $1
		         ORDER BY score DESC, source_count DESC, first_seen_at DESC
		         LIMIT $3
		         FOR UPDATE SKIP LOCKED)`,
		since, minScore, n, now)
	if err != nil {
		return 0, err
	}
	c, err := res.RowsAffected()
	return int(c), err
}

func (r *NewsRepository) Prune(ctx context.Context, keep int, candidatesBefore, logsBefore time.Time) (int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// The rolling window: published stories beyond the newest `keep` go,
	// with their items through the cascade. Pinned stories stay until
	// unpinned.
	res, err := tx.ExecContext(ctx,
		`DELETE FROM news_stories WHERE id IN (
		        SELECT id FROM news_stories
		         WHERE published_at IS NOT NULL AND NOT pinned
		         ORDER BY published_at DESC, id
		        OFFSET $1)`, keep)
	if err != nil {
		return 0, err
	}
	published, _ := res.RowsAffected()

	res, err = tx.ExecContext(ctx,
		`DELETE FROM news_stories
		  WHERE published_at IS NULL AND NOT pinned AND created_at < $1`, candidatesBefore)
	if err != nil {
		return 0, err
	}
	candidates, _ := res.RowsAffected()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM news_runs WHERE status <> 'running' AND started_at < $1`, logsBefore); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM news_usage WHERE day < $1::date`, logsBefore); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(published + candidates), nil
}

// --- reads ---------------------------------------------------------------------

func (r *NewsRepository) ListPublished(ctx context.Context, category *news.Category, limit, offset int) ([]news.Story, error) {
	var out []news.Story
	var cat *string
	if category != nil {
		c := string(*category)
		cat = &c
	}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+storyColumns+` FROM news_stories
		  WHERE published_at IS NOT NULL AND NOT hidden
		    AND ($1::text IS NULL OR category = $1::text)
		  ORDER BY pinned DESC, published_at DESC, id
		  LIMIT $2 OFFSET $3`, cat, limit, offset)
	return out, err
}

func (r *NewsRepository) Top(ctx context.Context, since time.Time, limit int) ([]news.Story, error) {
	var out []news.Story
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+storyColumns+` FROM news_stories
		  WHERE published_at IS NOT NULL AND NOT hidden
		    AND (pinned OR published_at >= $1)
		  ORDER BY pinned DESC, score DESC, published_at DESC, id
		  LIMIT $2`, since, limit)
	return out, err
}

func (r *NewsRepository) ListAll(ctx context.Context, limit, offset int) ([]news.AdminStory, error) {
	rows, err := r.db.QueryxContext(ctx,
		`SELECT s.*, COALESCE(
		        (SELECT array_agg(DISTINCT i.source_key) FROM news_items i WHERE i.story_id = s.id),
		        '{}') AS sources
		   FROM (SELECT `+storyColumns+` FROM news_stories
		          ORDER BY pinned DESC, COALESCE(published_at, first_seen_at) DESC, id
		          LIMIT $1 OFFSET $2) s
		  ORDER BY s.pinned DESC, COALESCE(s.published_at, s.first_seen_at) DESC, s.id`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []news.AdminStory
	for rows.Next() {
		var a news.AdminStory
		var sources pq.StringArray
		if err := rows.Scan(&a.ID, &a.Headline, &a.URL, &a.SourceName, &a.Attribution,
			&a.Summary, &a.Category, &a.Score, &a.Reason, &a.Scorer, &a.SourceCount,
			&a.ScoredSources, &a.FirstSeenAt, &a.PublishedAt, &a.Hidden, &a.Pinned,
			&a.CreatedAt, &a.UpdatedAt, &sources); err != nil {
			return nil, err
		}
		a.Sources = []string(sources)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *NewsRepository) SetFlags(ctx context.Context, id string, hidden, pinned *bool, now time.Time) (*news.Story, error) {
	var s news.Story
	err := r.db.GetContext(ctx, &s,
		`UPDATE news_stories
		    SET hidden = COALESCE($2, hidden),
		        pinned = COALESCE($3, pinned),
		        published_at = CASE WHEN COALESCE($3, FALSE) AND published_at IS NULL
		                            THEN $4 ELSE published_at END,
		        updated_at = NOW()
		  WHERE id = $1
		  RETURNING `+storyColumns, id, hidden, pinned, now)
	if err != nil {
		return nil, translate("story", err)
	}
	return &s, nil
}
