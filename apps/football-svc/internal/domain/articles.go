package domain

import (
	"encoding/json"
	"time"
)

// ArticleStatus is where an article is in the review workflow. See the
// 000010 migration for the diagram.
type ArticleStatus string

const (
	ArticleDraft            ArticleStatus = "draft"
	ArticleInReview         ArticleStatus = "in_review"
	ArticleChangesRequested ArticleStatus = "changes_requested"
	ArticlePublished        ArticleStatus = "published"
)

// Valid reports whether s is one of the four statuses.
func (s ArticleStatus) Valid() bool {
	switch s {
	case ArticleDraft, ArticleInReview, ArticleChangesRequested, ArticlePublished:
		return true
	}
	return false
}

// Article is a piece of football analysis.
//
// Readers see it once it is published and its publish_at has passed. Until
// then it lives on the editorial desk, where its writer and the
// administrators can reach it by id.
type Article struct {
	ID         string `db:"id" json:"id"`
	Slug       string `db:"slug" json:"slug"`
	AuthorID   string `db:"author_id" json:"author_id"`
	AuthorName string `db:"author_name" json:"author_name"`

	Title        string `db:"title" json:"title"`
	Subtitle     string `db:"subtitle" json:"subtitle"`
	CoverURL     string `db:"cover_url" json:"cover_url"`
	CoverCaption string `db:"cover_caption" json:"cover_caption"`
	// Body is the editor's document. The service checks every node and link
	// in it on the way in; the reader renders only the node types it knows.
	Body           json.RawMessage `db:"body" json:"body"`
	Excerpt        string          `db:"excerpt" json:"excerpt"`
	ReadingMinutes int             `db:"reading_minutes" json:"reading_minutes"`
	Tags           StringList      `db:"tags" json:"tags"`

	Status           ArticleStatus `db:"status" json:"status"`
	PublishAt        *time.Time    `db:"publish_at" json:"publish_at"`
	FirstPublishedAt *time.Time    `db:"first_published_at" json:"first_published_at"`
	SubmittedAt      *time.Time    `db:"submitted_at" json:"submitted_at"`

	ReviewerID   *string    `db:"reviewer_id" json:"reviewer_id"`
	ReviewerName string     `db:"reviewer_name" json:"reviewer_name"`
	ReviewedAt   *time.Time `db:"reviewed_at" json:"reviewed_at"`
	ReviewNote   string     `db:"review_note" json:"review_note"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Live reports whether readers can see the article at now.
func (a *Article) Live(now time.Time) bool {
	return a.Status == ArticlePublished && a.PublishAt != nil && !a.PublishAt.After(now)
}

// ForReaders strips what only the desk should see: who reviewed it and what
// they said. The review note is a conversation between writer and editor,
// not part of the piece.
func (a *Article) ForReaders() *Article {
	out := *a
	out.ReviewerID = nil
	out.ReviewerName = ""
	out.ReviewedAt = nil
	out.ReviewNote = ""
	out.SubmittedAt = nil
	return &out
}

// ArticleSummary is an article without its body, for lists.
type ArticleSummary struct {
	ID             string        `db:"id" json:"id"`
	Slug           string        `db:"slug" json:"slug"`
	AuthorID       string        `db:"author_id" json:"author_id"`
	AuthorName     string        `db:"author_name" json:"author_name"`
	Title          string        `db:"title" json:"title"`
	Subtitle       string        `db:"subtitle" json:"subtitle"`
	CoverURL       string        `db:"cover_url" json:"cover_url"`
	Excerpt        string        `db:"excerpt" json:"excerpt"`
	ReadingMinutes int           `db:"reading_minutes" json:"reading_minutes"`
	Tags           StringList    `db:"tags" json:"tags"`
	Status         ArticleStatus `db:"status" json:"status"`
	PublishAt      *time.Time    `db:"publish_at" json:"publish_at"`
	SubmittedAt    *time.Time    `db:"submitted_at" json:"submitted_at"`
	ReviewNote     string        `db:"review_note" json:"review_note"`
	CreatedAt      time.Time     `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time     `db:"updated_at" json:"updated_at"`
}

// ArticleDeskFilter narrows the editorial desk's list.
type ArticleDeskFilter struct {
	// AuthorID limits it to one writer. Always set for an editor, who sees
	// only their own; optional for an administrator.
	AuthorID *string
	Status   *ArticleStatus
}
