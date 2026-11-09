package news

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
)

// Service is what the HTTP handlers call: the public reads, briefly cached,
// and the administrator's moderation.
type Service struct {
	Store    Store
	Pipeline *Pipeline
	Now      func() time.Time
	// CacheTTL is how long a public read is served from memory.
	CacheTTL time.Duration

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	value   any
	expires time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) cachedRead(key string, load func() (any, error)) (any, error) {
	ttl := s.CacheTTL
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	now := s.now()
	s.mu.Lock()
	if c, ok := s.cache[key]; ok && now.Before(c.expires) {
		s.mu.Unlock()
		return c.value, nil
	}
	s.mu.Unlock()

	v, err := load()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.cache == nil || len(s.cache) > 256 {
		s.cache = map[string]cached{}
	}
	s.cache[key] = cached{value: v, expires: now.Add(ttl)}
	s.mu.Unlock()
	return v, nil
}

// Invalidate drops cached reads, after a moderation change or a run.
func (s *Service) Invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

// Page is one page of the public feed.
type Page struct {
	Items   []PublicStory `json:"items"`
	Limit   int           `json:"limit"`
	Offset  int           `json:"offset"`
	HasMore bool          `json:"has_more"`
}

func public(stories []Story) []PublicStory {
	out := make([]PublicStory, len(stories))
	for i, st := range stories {
		out[i] = st.Public()
	}
	return out
}

// List returns published, visible stories: pinned first, then newest.
func (s *Service) List(ctx context.Context, category string, limit, offset int) (Page, error) {
	var cat *Category
	if category != "" {
		c, ok := ParseCategory(category)
		if !ok {
			return Page{}, apperr.Invalid("unknown category")
		}
		cat = &c
	}
	limit = min(max(limit, 1), 50)
	offset = max(offset, 0)
	key := "list:" + category + ":" + strconv.Itoa(limit) + ":" + strconv.Itoa(offset)
	v, err := s.cachedRead(key, func() (any, error) {
		rows, err := s.Store.ListPublished(ctx, cat, limit+1, offset)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		return Page{Items: public(rows), Limit: limit, Offset: offset, HasMore: more}, nil
	})
	if err != nil {
		return Page{}, err
	}
	return v.(Page), nil
}

// Top returns up to limit of the highest-scoring stories published in the
// last 24 hours, pinned ones first whatever their age.
func (s *Service) Top(ctx context.Context, limit int) ([]PublicStory, error) {
	limit = min(max(limit, 1), 5)
	v, err := s.cachedRead("top:"+strconv.Itoa(limit), func() (any, error) {
		rows, err := s.Store.Top(ctx, s.now().Add(-24*time.Hour), limit)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		return public(rows), nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]PublicStory), nil
}

func requireAdmin(ctx context.Context) error {
	c, ok := auth.GetClaims(ctx)
	if !ok {
		return apperr.Unauthorized("authentication required")
	}
	if c.Role != "admin" {
		return apperr.Forbidden("insufficient permissions")
	}
	return nil
}

// AdminOverview is everything the admin view shows.
type AdminOverview struct {
	Items   []AdminStory `json:"items"`
	HasMore bool         `json:"has_more"`
	Runs    []RunReport  `json:"runs"`
	Sources []SourceInfo `json:"sources"`
}

// SourceInfo is a registry entry as the admin view shows it.
type SourceInfo struct {
	Key                   string `json:"key"`
	Name                  string `json:"name"`
	Enabled               bool   `json:"enabled"`
	CommercialUseVerified bool   `json:"commercial_use_verified"`
	TermsURL              string `json:"terms_url"`
}

// AdminList returns every stored story, published or not, newest first.
func (s *Service) AdminList(ctx context.Context, limit, offset int) (AdminOverview, error) {
	if err := requireAdmin(ctx); err != nil {
		return AdminOverview{}, err
	}
	limit = min(max(limit, 1), 100)
	rows, err := s.Store.ListAll(ctx, limit+1, max(offset, 0))
	if err != nil {
		return AdminOverview{}, apperr.Internal(err)
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	if rows == nil {
		rows = []AdminStory{}
	}
	runs, err := s.Store.LastRuns(ctx, 10)
	if err != nil {
		return AdminOverview{}, apperr.Internal(err)
	}
	if runs == nil {
		runs = []RunReport{}
	}
	var sources []SourceInfo
	registry := Registry
	if s.Pipeline != nil {
		registry = s.Pipeline.Sources
	}
	for _, src := range registry {
		sources = append(sources, SourceInfo{
			Key: src.Key, Name: src.Name, Enabled: src.Enabled,
			CommercialUseVerified: src.CommercialUseVerified, TermsURL: src.TermsURL,
		})
	}
	return AdminOverview{Items: rows, HasMore: more, Runs: runs, Sources: sources}, nil
}

// SetFlags hides or pins a story. Pinning a story that was never published
// publishes it.
func (s *Service) SetFlags(ctx context.Context, id string, hidden, pinned *bool) (*Story, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if hidden == nil && pinned == nil {
		return nil, apperr.Invalid("nothing to change")
	}
	st, err := s.Store.SetFlags(ctx, id, hidden, pinned, s.now())
	if err != nil {
		return nil, err
	}
	s.Invalidate()
	return st, nil
}

// RunNow runs the pipeline at once, for an administrator.
func (s *Service) RunNow(ctx context.Context) (RunReport, error) {
	if err := requireAdmin(ctx); err != nil {
		return RunReport{}, err
	}
	if s.Pipeline == nil {
		return RunReport{Skipped: "the news job is disabled"}, nil
	}
	// Detached from the request, so a closed browser tab does not cut a run
	// off halfway; the pipeline has its own timeout.
	report, err := s.Pipeline.Run(context.WithoutCancel(ctx))
	s.Invalidate()
	if err != nil {
		return report, apperr.Internal(err)
	}
	return report, nil
}
