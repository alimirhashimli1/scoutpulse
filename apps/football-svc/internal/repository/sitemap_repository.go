package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// SitemapEntry is one public page: the key that goes in its URL (an id, or an
// article's slug) and, where the table records one, when it last changed.
type SitemapEntry struct {
	Key     string     `db:"key" json:"key"`
	LastMod *time.Time `db:"lastmod" json:"lastmod,omitempty"`
}

// SitemapKinds are the entity kinds a sitemap lists, mapped to the query that
// lists them. Only public records: an article is included once it is live,
// exactly the rule GET /articles/{slug} applies.
//
// Ordered by a stable key so offset paging never skips or repeats a row
// between the sitemap files a crawler fetches minutes apart.
var sitemapQueries = map[string]string{
	"player":  `SELECT id::text AS key, NULL::timestamptz AS lastmod FROM players ORDER BY id LIMIT $1 OFFSET $2`,
	"team":    `SELECT id::text AS key, NULL::timestamptz AS lastmod FROM teams ORDER BY id LIMIT $1 OFFSET $2`,
	"league":  `SELECT id::text AS key, NULL::timestamptz AS lastmod FROM leagues ORDER BY id LIMIT $1 OFFSET $2`,
	"coach":   `SELECT id::text AS key, NULL::timestamptz AS lastmod FROM coaches ORDER BY id LIMIT $1 OFFSET $2`,
	"article": `SELECT slug AS key, updated_at AS lastmod FROM articles WHERE status = 'published' AND publish_at <= NOW() ORDER BY id LIMIT $1 OFFSET $2`,
}

var sitemapCounts = map[string]string{
	"player":  `SELECT COUNT(*) FROM players`,
	"team":    `SELECT COUNT(*) FROM teams`,
	"league":  `SELECT COUNT(*) FROM leagues`,
	"coach":   `SELECT COUNT(*) FROM coaches`,
	"article": `SELECT COUNT(*) FROM articles WHERE status = 'published' AND publish_at <= NOW()`,
}

// SitemapRepository lists what a search engine should be told about.
type SitemapRepository struct {
	db *sqlx.DB
}

func NewSitemapRepository(db *sqlx.DB) *SitemapRepository {
	return &SitemapRepository{db: db}
}

// IsSitemapKind reports whether kind names a listable entity.
func IsSitemapKind(kind string) bool {
	_, ok := sitemapQueries[kind]
	return ok
}

// Counts returns how many public records of each kind exist, so the caller
// can work out how many sitemap files to publish.
func (r *SitemapRepository) Counts(ctx context.Context) (map[string]int, error) {
	out := make(map[string]int, len(sitemapCounts))
	for kind, q := range sitemapCounts {
		var n int
		if err := r.db.GetContext(ctx, &n, q); err != nil {
			return nil, fmt.Errorf("count %s: %w", kind, err)
		}
		out[kind] = n
	}
	return out, nil
}

// List returns one slice of kind's entries.
func (r *SitemapRepository) List(ctx context.Context, kind string, limit, offset int) ([]SitemapEntry, error) {
	q, ok := sitemapQueries[kind]
	if !ok {
		return nil, fmt.Errorf("unknown sitemap kind %q", kind)
	}
	entries := []SitemapEntry{}
	if err := r.db.SelectContext(ctx, &entries, q, limit, offset); err != nil {
		return nil, fmt.Errorf("list %s: %w", kind, err)
	}
	return entries, nil
}
