package news

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"
)

// memStore is an in-memory Store with the same semantics as the Postgres one
// where the pipeline depends on them.
type memStore struct {
	mu      sync.Mutex
	stories map[string]*Story
	items   map[string]storedItem // by canonical
	usage   map[string]int
	running bool
	runs    []RunReport
	nextID  int
}

type storedItem struct {
	story string
	Article
}

func newMemStore() *memStore {
	return &memStore{stories: map[string]*Story{}, items: map[string]storedItem{}, usage: map[string]int{}}
}

func (m *memStore) Usage(_ context.Context, day time.Time, meter string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.usage[day.Format("2006-01-02")+meter], nil
}

func (m *memStore) AddUsage(_ context.Context, day time.Time, meter string, n int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := day.Format("2006-01-02") + meter
	m.usage[k] = max(0, m.usage[k]+n)
	return nil
}

func (m *memStore) BeginRun(context.Context, time.Duration) (int64, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return 0, false, nil
	}
	m.running = true
	return int64(len(m.runs) + 1), true, nil
}

func (m *memStore) FinishRun(_ context.Context, r RunReport) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	m.runs = append(m.runs, r)
	return nil
}

func (m *memStore) KnownCanonicals(_ context.Context, cs []string) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for _, c := range cs {
		if _, ok := m.items[c]; ok {
			out[c] = true
		}
	}
	return out, nil
}

func (m *memStore) RecentStories(_ context.Context, since time.Time) ([]StoryRef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []StoryRef
	for _, s := range m.stories {
		if s.FirstSeenAt.Before(since) {
			continue
		}
		ref := StoryRef{ID: s.ID, Headline: s.Headline, Published: s.PublishedAt != nil}
		keys := map[string]bool{}
		for _, it := range m.items {
			if it.story == s.ID {
				ref.Titles = append(ref.Titles, it.Title)
				if !keys[it.SourceKey] {
					keys[it.SourceKey] = true
					ref.SourceKeys = append(ref.SourceKeys, it.SourceKey)
				}
			}
		}
		out = append(out, ref)
	}
	return out, nil
}

func (m *memStore) CreateStory(_ context.Context, s *Story, items []Article) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	s.ID = "s" + strconv.Itoa(m.nextID)
	s.CreatedAt = time.Now()
	cp := *s
	m.stories[s.ID] = &cp
	for _, it := range items {
		if _, ok := m.items[it.Canonical]; !ok {
			m.items[it.Canonical] = storedItem{story: s.ID, Article: it}
		}
	}
	return nil
}

func (m *memStore) AttachItems(_ context.Context, id string, items []Article, outlets int, v *Verdict) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.stories[id]
	if !ok {
		return errors.New("no story")
	}
	for _, it := range items {
		if _, ok := m.items[it.Canonical]; !ok {
			m.items[it.Canonical] = storedItem{story: id, Article: it}
		}
	}
	s.SourceCount = max(s.SourceCount, outlets)
	if v != nil {
		s.Score, s.Category, s.Reason, s.Scorer = v.Score, v.Category, v.Reason, v.Scorer
		s.ScoredSources = outlets
		if v.Summary != "" {
			s.Summary = v.Summary
		}
	}
	return nil
}

func (m *memStore) PublishedSince(_ context.Context, t time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.stories {
		if s.PublishedAt != nil && !s.PublishedAt.Before(t) {
			n++
		}
	}
	return n, nil
}

func (m *memStore) PublishBest(_ context.Context, since time.Time, minScore, n int, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var c []*Story
	for _, s := range m.stories {
		if s.PublishedAt == nil && !s.Hidden && s.Score >= minScore && !s.FirstSeenAt.Before(since) {
			c = append(c, s)
		}
	}
	sort.Slice(c, func(i, j int) bool { return c[i].Score > c[j].Score })
	if len(c) > n {
		c = c[:n]
	}
	for _, s := range c {
		t := now
		s.PublishedAt = &t
	}
	return len(c), nil
}

func (m *memStore) Prune(_ context.Context, keep int, before, _ time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var pub []*Story
	n := 0
	for id, s := range m.stories {
		switch {
		case s.Pinned:
		case s.PublishedAt != nil:
			pub = append(pub, s)
		case s.CreatedAt.Before(before):
			m.drop(id)
			n++
		}
	}
	sort.Slice(pub, func(i, j int) bool { return pub[i].PublishedAt.After(*pub[j].PublishedAt) })
	for i := keep; i < len(pub); i++ {
		m.drop(pub[i].ID)
		n++
	}
	return n, nil
}

func (m *memStore) drop(id string) {
	delete(m.stories, id)
	for k, it := range m.items {
		if it.story == id {
			delete(m.items, k)
		}
	}
}

func (m *memStore) published() []Story {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Story
	for _, s := range m.stories {
		if s.PublishedAt != nil {
			out = append(out, *s)
		}
	}
	return out
}

func (m *memStore) ListPublished(context.Context, *Category, int, int) ([]Story, error) {
	return m.published(), nil
}
func (m *memStore) Top(context.Context, time.Time, int) ([]Story, error) { return m.published(), nil }
func (m *memStore) ListAll(context.Context, int, int) ([]AdminStory, error) {
	return nil, nil
}
func (m *memStore) SetFlags(context.Context, string, *bool, *bool, time.Time) (*Story, error) {
	return nil, nil
}
func (m *memStore) LastRuns(context.Context, int) ([]RunReport, error) { return m.runs, nil }
