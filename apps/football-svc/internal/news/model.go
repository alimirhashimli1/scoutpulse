// Package news collects football headlines from licensed feeds, groups them
// into stories, scores each story for how much a scout would care, and
// publishes the best few a day.
//
// What is kept is deliberately small: the original headline, a short summary
// the scorer wrote, the source, the time and a link back to the outlet. Feed
// descriptions are read in memory to write that summary and are never stored,
// and nothing from an article page is ever fetched.
package news

import (
	"strings"
	"time"
)

// Category is one of the six sections a story can be filed under.
type Category string

const (
	CategoryTransfers    Category = "transfers"
	CategoryInjuries     Category = "injuries"
	CategoryResults      Category = "results"
	CategoryManagers     Category = "managers"
	CategoryYoungTalents Category = "young_talents"
	CategoryOther        Category = "other"
)

// Categories lists every valid category, in the order the UI shows them.
var Categories = []Category{
	CategoryTransfers, CategoryInjuries, CategoryResults,
	CategoryManagers, CategoryYoungTalents, CategoryOther,
}

// ParseCategory accepts a category in the forms a model tends to write it --
// "Young Talents", "young-talents", "TRANSFER" -- and reports whether it is
// one of the six.
func ParseCategory(s string) (Category, bool) {
	k := strings.ToLower(strings.TrimSpace(s))
	k = strings.NewReplacer(" ", "_", "-", "_").Replace(k)
	switch k {
	case "transfer":
		k = "transfers"
	case "injury":
		k = "injuries"
	case "result":
		k = "results"
	case "manager":
		k = "managers"
	case "young_talent", "youngtalents", "youth":
		k = "young_talents"
	}
	for _, c := range Categories {
		if string(c) == k {
			return c, true
		}
	}
	return "", false
}

// Publishing rules.
const (
	// PublishThreshold is the lowest score that is published.
	PublishThreshold = 7
	// MaxPublishedPerDay caps publishing per UTC day, pins included.
	MaxPublishedPerDay = 10
	// KeepPublished is the size of the rolling feed. Older published stories
	// are deleted, pinned ones excepted.
	KeepPublished = 100
	// IngestWindow is how old an item may be and still be taken in. It is
	// also what makes deletion safe: nothing deleted is young enough to be
	// fetched again and come back.
	IngestWindow = 36 * time.Hour
	// CandidateTTL is how long a story that was never published is kept, for
	// grouping later items with it and for the admin view.
	CandidateTTL = 72 * time.Hour
	// RunLogTTL is how long the run log is kept.
	RunLogTTL = 14 * 24 * time.Hour
)

// Article is one item from one feed, normalised. It lives only for the
// length of a run; Description in particular is never written anywhere.
type Article struct {
	// SourceKey identifies the outlet, for counting how many distinct
	// outlets carried a story: "bbc-sport", or "newsdata:espn".
	SourceKey   string
	SourceName  string
	Attribution string
	// Supporting marks coverage that only corroborates -- NewsData's free
	// tier is about 12 hours behind, so it never leads a story.
	Supporting  bool
	Title       string
	Description string
	URL         string
	Canonical   string
	PublishedAt time.Time
}

// Story is a published or candidate story as stored and served.
type Story struct {
	ID            string     `db:"id" json:"id"`
	Headline      string     `db:"headline" json:"headline"`
	URL           string     `db:"url" json:"url"`
	SourceName    string     `db:"source_name" json:"source_name"`
	Attribution   string     `db:"attribution" json:"attribution"`
	Summary       string     `db:"summary" json:"summary"`
	Category      Category   `db:"category" json:"category"`
	Score         int        `db:"score" json:"score"`
	Reason        string     `db:"reason" json:"reason,omitempty"`
	Scorer        string     `db:"scorer" json:"scorer,omitempty"`
	SourceCount   int        `db:"source_count" json:"source_count"`
	ScoredSources int        `db:"scored_sources" json:"-"`
	FirstSeenAt   time.Time  `db:"first_seen_at" json:"first_seen_at"`
	PublishedAt   *time.Time `db:"published_at" json:"published_at"`
	Hidden        bool       `db:"hidden" json:"hidden"`
	Pinned        bool       `db:"pinned" json:"pinned"`
	CreatedAt     time.Time  `db:"created_at" json:"-"`
	UpdatedAt     time.Time  `db:"updated_at" json:"-"`
}

// PublicStory is what anonymous readers get: no scoring internals.
type PublicStory struct {
	ID          string    `json:"id"`
	Headline    string    `json:"headline"`
	URL         string    `json:"url"`
	SourceName  string    `json:"source_name"`
	Attribution string    `json:"attribution"`
	Summary     string    `json:"summary"`
	Category    Category  `json:"category"`
	Score       int       `json:"score"`
	SourceCount int       `json:"source_count"`
	PublishedAt time.Time `json:"published_at"`
	Pinned      bool      `json:"pinned"`
}

// Public strips a story down to its public fields.
func (s Story) Public() PublicStory {
	p := PublicStory{
		ID: s.ID, Headline: s.Headline, URL: s.URL, SourceName: s.SourceName,
		Attribution: s.Attribution, Summary: s.Summary, Category: s.Category,
		Score: s.Score, SourceCount: s.SourceCount, Pinned: s.Pinned,
	}
	if s.PublishedAt != nil {
		p.PublishedAt = *s.PublishedAt
	}
	return p
}

// AdminStory adds the stored sources to a story for the admin view.
type AdminStory struct {
	Story
	Sources []string `json:"sources"`
}

// StoryRef is what grouping needs to know about a recent stored story.
type StoryRef struct {
	ID         string
	Titles     []string
	SourceKeys []string
	Published  bool
	Headline   string
}

// Verdict is a scorer's judgement of one story.
type Verdict struct {
	Score    int
	Category Category
	Reason   string
	Summary  string
	Scorer   string
}

// RunReport is what one run did, for the log and the admin trigger.
type RunReport struct {
	ID        int64     `json:"id"`
	Status    string    `json:"status"`
	Skipped   string    `json:"skipped,omitempty"`
	Fetched   int       `json:"fetched"`
	NewItems  int       `json:"new_items"`
	Scored    int       `json:"scored"`
	Published int       `json:"published"`
	Pruned    int       `json:"pruned"`
	AINeurons int       `json:"ai_neurons"`
	Errors    []string  `json:"errors"`
	StartedAt time.Time `json:"started_at"`
}
