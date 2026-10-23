package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/platform/apperr"
)

// ArticleRepository stores analysis articles.
type ArticleRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Article, error)
	// GetLive returns the article at slug only if readers can see it at now:
	// published, and its publish_at reached. Anything else is not found, so a
	// draft's slug answers exactly as a mistyped one does.
	GetLive(ctx context.Context, slug string, now time.Time) (*domain.Article, error)
	// ListLive returns live articles, newest first, optionally with one tag.
	ListLive(ctx context.Context, tag *string, now time.Time, page domain.Page) ([]domain.ArticleSummary, error)
	// ListDesk returns articles in any state, most recently changed first.
	ListDesk(ctx context.Context, filter domain.ArticleDeskFilter, page domain.Page) ([]domain.ArticleSummary, error)
	Create(ctx context.Context, article *domain.Article) error
	// Update writes every mutable column. The service decides what changed;
	// this only persists the result.
	Update(ctx context.Context, article *domain.Article) error
	Delete(ctx context.Context, id string) error
	CountByAuthor(ctx context.Context, authorID string) (int, error)
	// DeleteUnpublishedByAuthor removes a deleted account's drafts and
	// submissions. Published pieces stay: they are part of the site, and the
	// byline was captured when they were written.
	DeleteUnpublishedByAuthor(ctx context.Context, authorID string) (int64, error)
}

type postgresArticleRepository struct {
	db *sqlx.DB
}

func NewPostgresArticleRepository(db *sqlx.DB) ArticleRepository {
	return &postgresArticleRepository{db: db}
}

const articleColumns = `id, slug, author_id, author_name, title, subtitle, cover_url, cover_caption,
	body, excerpt, reading_minutes, tags, status, publish_at, first_published_at, submitted_at,
	reviewer_id, reviewer_name, reviewed_at, review_note, created_at, updated_at`

const articleSummaryColumns = `id, slug, author_id, author_name, title, subtitle, cover_url,
	excerpt, reading_minutes, tags, status, publish_at, submitted_at, review_note,
	created_at, updated_at`

func (r *postgresArticleRepository) GetByID(ctx context.Context, id string) (*domain.Article, error) {
	var a domain.Article
	query := `SELECT ` + articleColumns + ` FROM articles WHERE id = $1`
	if err := r.db.GetContext(ctx, &a, query, id); err != nil {
		return nil, translate("article", err)
	}
	return &a, nil
}

func (r *postgresArticleRepository) GetLive(ctx context.Context, slug string, now time.Time) (*domain.Article, error) {
	var a domain.Article
	query := `SELECT ` + articleColumns + ` FROM articles
	           WHERE slug = $1 AND status = 'published' AND publish_at <= $2`
	if err := r.db.GetContext(ctx, &a, query, slug, now); err != nil {
		return nil, translate("article", err)
	}
	return &a, nil
}

// ListLive reads the partial index on live rows. The tag test is written as
// array containment so the GIN index on tags can serve it.
func (r *postgresArticleRepository) ListLive(ctx context.Context, tag *string, now time.Time, page domain.Page) ([]domain.ArticleSummary, error) {
	var out []domain.ArticleSummary
	query := `SELECT ` + articleSummaryColumns + ` FROM articles
	           WHERE status = 'published' AND publish_at <= $1
	             AND ($2::text IS NULL OR tags @> ARRAY[$2::text])
	           ORDER BY publish_at DESC, id
	           LIMIT $3 OFFSET $4`
	if err := r.db.SelectContext(ctx, &out, query, now, tag, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("article", err)
	}
	return out, nil
}

func (r *postgresArticleRepository) ListDesk(ctx context.Context, filter domain.ArticleDeskFilter, page domain.Page) ([]domain.ArticleSummary, error) {
	var out []domain.ArticleSummary
	var status *string
	if filter.Status != nil {
		s := string(*filter.Status)
		status = &s
	}
	query := `SELECT ` + articleSummaryColumns + ` FROM articles
	           WHERE ($1::uuid IS NULL OR author_id = $1::uuid)
	             AND ($2::text IS NULL OR status = $2::text)
	           ORDER BY updated_at DESC, id
	           LIMIT $3 OFFSET $4`
	if err := r.db.SelectContext(ctx, &out, query, filter.AuthorID, status, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("article", err)
	}
	return out, nil
}

// Create inserts a draft. A slug collision surfaces as a conflict, which the
// service answers by drawing another suffix.
//
// The body is passed as a string: lib/pq sends a []byte as bytea, which
// Postgres will not read as JSON.
func (r *postgresArticleRepository) Create(ctx context.Context, a *domain.Article) error {
	query := `INSERT INTO articles (slug, author_id, author_name, title, subtitle, cover_url,
	                                cover_caption, body, excerpt, reading_minutes, tags, status)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	          RETURNING id, created_at, updated_at`
	err := r.db.QueryRowContext(ctx, query,
		a.Slug, a.AuthorID, a.AuthorName, a.Title, a.Subtitle, a.CoverURL, a.CoverCaption,
		string(a.Body), a.Excerpt, a.ReadingMinutes, a.Tags, string(a.Status),
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return translate("article", err)
	}
	return nil
}

func (r *postgresArticleRepository) Update(ctx context.Context, a *domain.Article) error {
	query := `UPDATE articles
	             SET slug = $1, title = $2, subtitle = $3, cover_url = $4, cover_caption = $5,
	                 body = $6, excerpt = $7, reading_minutes = $8, tags = $9, status = $10,
	                 publish_at = $11, first_published_at = $12, submitted_at = $13,
	                 reviewer_id = $14, reviewer_name = $15, reviewed_at = $16, review_note = $17,
	                 updated_at = NOW()
	           WHERE id = $18
	       RETURNING updated_at`
	err := r.db.QueryRowContext(ctx, query,
		a.Slug, a.Title, a.Subtitle, a.CoverURL, a.CoverCaption,
		string(a.Body), a.Excerpt, a.ReadingMinutes, a.Tags, string(a.Status),
		a.PublishAt, a.FirstPublishedAt, a.SubmittedAt,
		a.ReviewerID, a.ReviewerName, a.ReviewedAt, a.ReviewNote,
		a.ID,
	).Scan(&a.UpdatedAt)
	if err != nil {
		return translate("article", err)
	}
	return nil
}

func (r *postgresArticleRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM articles WHERE id = $1`, id)
	return affected("article", res, err)
}

func (r *postgresArticleRepository) CountByAuthor(ctx context.Context, authorID string) (int, error) {
	var n int
	if err := r.db.GetContext(ctx, &n, `SELECT count(*) FROM articles WHERE author_id = $1`, authorID); err != nil {
		return 0, translate("article", err)
	}
	return n, nil
}

func (r *postgresArticleRepository) DeleteUnpublishedByAuthor(ctx context.Context, authorID string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM articles WHERE author_id = $1 AND status <> 'published'`, authorID)
	if err != nil {
		return 0, apperr.Wrap(apperr.KindInternal, "could not remove the account's draft articles", err)
	}
	return res.RowsAffected()
}
