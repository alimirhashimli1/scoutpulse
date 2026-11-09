package news

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// Store is the persistence the pipeline and the service need.
type Store interface {
	UsageStore

	// BeginRun claims the single running slot. ok is false when another run
	// holds it; a slot left by a run that died is reclaimed after staleAfter.
	BeginRun(ctx context.Context, staleAfter time.Duration) (id int64, ok bool, err error)
	FinishRun(ctx context.Context, report RunReport) error

	// KnownCanonicals reports which of these links are already stored.
	KnownCanonicals(ctx context.Context, canonicals []string) (map[string]bool, error)
	// RecentStories returns stories first seen since, with their item titles.
	RecentStories(ctx context.Context, since time.Time) ([]StoryRef, error)
	// CreateStory stores a new story and its items in one transaction.
	CreateStory(ctx context.Context, story *Story, items []Article) error
	// AttachItems adds items to a stored story, updates its outlet count and,
	// when v is given, its verdict. Items already stored are skipped.
	AttachItems(ctx context.Context, storyID string, items []Article, outlets int, v *Verdict) error

	// PublishedSince counts stories published at or after t.
	PublishedSince(ctx context.Context, t time.Time) (int, error)
	// PublishBest publishes up to n unhidden candidates first seen since,
	// scoring at least minScore, best first. It returns how many it published.
	PublishBest(ctx context.Context, since time.Time, minScore, n int, now time.Time) (int, error)
	// Prune keeps the newest keep published stories (pinned ones aside),
	// deletes unpublished ones created before candidatesBefore, and trims
	// runs and usage older than logsBefore. It returns stories deleted.
	Prune(ctx context.Context, keep int, candidatesBefore, logsBefore time.Time) (int, error)

	ListPublished(ctx context.Context, category *Category, limit, offset int) ([]Story, error)
	Top(ctx context.Context, since time.Time, limit int) ([]Story, error)
	ListAll(ctx context.Context, limit, offset int) ([]AdminStory, error)
	SetFlags(ctx context.Context, id string, hidden, pinned *bool, now time.Time) (*Story, error)
	LastRuns(ctx context.Context, n int) ([]RunReport, error)
}

// Pipeline is one collect-score-publish run.
type Pipeline struct {
	Store   Store
	Sources []Source
	Fetcher Fetcher
	AI      AiScorer // nil means rules only
	Logger  *slog.Logger
	Now     func() time.Time
	// SourceTimeout bounds each source; RunTimeout bounds the whole run.
	SourceTimeout time.Duration
	RunTimeout    time.Duration
	// MaxAICalls bounds the AI calls in one run.
	MaxAICalls int

	mu sync.Mutex // overlapping runs within one process
}

const staleRunAfter = 20 * time.Minute

func (p *Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func (p *Pipeline) logger() *slog.Logger {
	if p.Logger != nil {
		return p.Logger
	}
	return slog.Default()
}

// Run performs one run. It never runs twice at once, in this process or
// across instances, and repeating it changes nothing that the first did:
// stored links are skipped, and the daily publishing cap is counted from the
// database.
func (p *Pipeline) Run(ctx context.Context) (RunReport, error) {
	if !p.mu.TryLock() {
		return RunReport{Skipped: "a run is already in progress"}, nil
	}
	defer p.mu.Unlock()

	active := Active(p.Sources)
	if len(active) == 0 {
		return RunReport{Skipped: "no source is both enabled and verified for commercial use"}, nil
	}

	timeout := p.RunTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	id, ok, err := p.Store.BeginRun(ctx, staleRunAfter)
	if err != nil {
		return RunReport{}, fmt.Errorf("begin run: %w", err)
	}
	if !ok {
		return RunReport{Skipped: "another instance is running"}, nil
	}
	report := RunReport{ID: id, StartedAt: p.now(), Errors: []string{}}
	runErr := p.run(ctx, active, &report)

	switch {
	case runErr != nil:
		report.Status = "failed"
		report.Errors = append(report.Errors, runErr.Error())
	case len(report.Errors) > 0:
		report.Status = "partial"
	default:
		report.Status = "ok"
	}
	// Recorded on a fresh context, so a timed-out run still frees the slot.
	finishCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer done()
	if err := p.Store.FinishRun(finishCtx, report); err != nil {
		p.logger().Error("news: could not record run", "run", id, "error", err)
	}
	p.logger().Info("news run",
		"run", id, "status", report.Status, "fetched", report.Fetched,
		"new_items", report.NewItems, "scored", report.Scored,
		"published", report.Published, "pruned", report.Pruned,
		"ai_neurons", report.AINeurons, "errors", len(report.Errors))
	return report, runErr
}

func (p *Pipeline) run(ctx context.Context, active []Source, report *RunReport) error {
	now := p.now()

	// 1. Fetch every source at once. A failure is logged and recorded, and
	//    the run goes on with the others.
	articles := p.fetchAll(ctx, active, report)
	report.Fetched = len(articles)

	// 2. Normalise: window, duplicates within the run, then links already
	//    stored by an earlier run.
	articles = Dedupe(InWindow(articles, now))
	links := make([]string, len(articles))
	for i, a := range articles {
		links[i] = a.Canonical
	}
	known, err := p.Store.KnownCanonicals(ctx, links)
	if err != nil {
		return fmt.Errorf("check stored links: %w", err)
	}
	fresh := articles[:0]
	for _, a := range articles {
		if !known[a.Canonical] {
			fresh = append(fresh, a)
		}
	}
	report.NewItems = len(fresh)

	// 3. Group into stories, joining recent stored ones where they match.
	if len(fresh) > 0 {
		recent, err := p.Store.RecentStories(ctx, now.Add(-CandidateTTL))
		if err != nil {
			return fmt.Errorf("load recent stories: %w", err)
		}
		groups := Cluster(fresh, recent)

		// 4. Score new stories, and unpublished ones that gained an outlet.
		if err := p.scoreAndStore(ctx, groups, now, report); err != nil {
			return err
		}
	}

	// 5. Publish the best candidates, within today's cap.
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	today, err := p.Store.PublishedSince(ctx, dayStart)
	if err != nil {
		return fmt.Errorf("count today's stories: %w", err)
	}
	if left := MaxPublishedPerDay - today; left > 0 {
		n, err := p.Store.PublishBest(ctx, now.Add(-IngestWindow), PublishThreshold, left, now)
		if err != nil {
			return fmt.Errorf("publish: %w", err)
		}
		report.Published = n
	}

	// 6. Keep the window rolling: the newest 100 published stories stay.
	pruned, err := p.Store.Prune(ctx, KeepPublished, now.Add(-CandidateTTL), now.Add(-RunLogTTL))
	if err != nil {
		return fmt.Errorf("prune: %w", err)
	}
	report.Pruned = pruned
	return nil
}

func (p *Pipeline) fetchAll(ctx context.Context, active []Source, report *RunReport) []Article {
	timeout := p.SourceTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	type result struct {
		src      Source
		articles []Article
		err      error
	}
	results := make(chan result, len(active))
	for _, src := range active {
		go func(src Source) {
			sctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			var r result
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						r.err = fmt.Errorf("panic: %v", rec)
					}
				}()
				r.articles, r.err = p.Fetcher.Fetch(sctx, src)
			}()
			r.src = src
			results <- r
		}(src)
	}
	var all []Article
	for range active {
		r := <-results
		if r.err != nil {
			p.logger().Warn("news source failed", "source", r.src.Key, "error", r.err)
			report.Errors = append(report.Errors, r.src.Key+": "+r.err.Error())
			continue
		}
		p.logger().Info("news source fetched", "source", r.src.Key, "items", len(r.articles))
		all = append(all, r.articles...)
	}
	return all
}

type pending struct {
	group     Group
	candidate Candidate
	outlets   int
	rescore   bool
}

func (p *Pipeline) scoreAndStore(ctx context.Context, groups []Group, now time.Time, report *RunReport) error {
	var work []pending
	for i, g := range groups {
		stored := []string(nil)
		published := false
		headline := Lead(g.Articles).Title
		if g.Existing != nil {
			stored = g.Existing.SourceKeys
			published = g.Existing.Published
			headline = g.Existing.Headline
		}
		outlets := Outlets(stored, g.Articles)
		desc := ""
		supportingOnly := true
		first := g.Articles[0].PublishedAt
		for _, a := range g.Articles {
			if desc == "" && !a.Supporting {
				desc = a.Description
			}
			if !a.Supporting {
				supportingOnly = false
			}
			if a.PublishedAt.Before(first) {
				first = a.PublishedAt
			}
		}
		if desc == "" {
			desc = g.Articles[0].Description
		}
		if g.Existing != nil && supportingOnly {
			// A stored story was led by a feed, unless every report so far
			// is supporting; the stored keys tell which.
			for _, k := range stored {
				if !isSupportingKey(k) {
					supportingOnly = false
				}
			}
		}
		rescore := g.Existing == nil || (!published && outlets > len(uniq(stored)))
		work = append(work, pending{
			group: g, outlets: outlets, rescore: rescore,
			candidate: Candidate{
				Key: strconv.Itoa(i), Headline: headline, Description: desc,
				Outlets: outlets, SupportingOnly: supportingOnly, FirstSeen: first,
			},
		})
	}

	verdicts := p.score(ctx, work, now, report)

	for _, w := range work {
		var v *Verdict
		if w.rescore {
			vv := verdicts[w.candidate.Key]
			v = &vv
			report.Scored++
		}
		if w.group.Existing != nil {
			if err := p.Store.AttachItems(ctx, w.group.StoryID, w.group.Articles, w.outlets, v); err != nil {
				return fmt.Errorf("attach items: %w", err)
			}
			continue
		}
		lead := Lead(w.group.Articles)
		story := &Story{
			Headline: lead.Title, URL: lead.URL, SourceName: lead.SourceName,
			Attribution: lead.Attribution, Summary: v.Summary, Category: v.Category,
			Score: v.Score, Reason: v.Reason, Scorer: v.Scorer,
			SourceCount: w.outlets, ScoredSources: w.outlets, FirstSeenAt: w.candidate.FirstSeen,
		}
		if err := p.Store.CreateStory(ctx, story, w.group.Articles); err != nil {
			return fmt.Errorf("store story: %w", err)
		}
	}
	return nil
}

// score asks the AI first, in batches, and fills every gap with rules: a
// failed call, a spent budget, or a verdict that did not validate.
func (p *Pipeline) score(ctx context.Context, work []pending, now time.Time, report *RunReport) map[string]Verdict {
	out := map[string]Verdict{}
	var todo []Candidate
	for _, w := range work {
		if w.rescore {
			todo = append(todo, w.candidate)
		}
	}

	if p.AI != nil {
		maxCalls := p.MaxAICalls
		if maxCalls <= 0 {
			maxCalls = 4
		}
		var before int
		if wa, ok := p.AI.(*WorkersAI); ok && wa.Budget != nil {
			before, _ = wa.Budget.Remaining(ctx)
		}
		for i, calls := 0, 0; i < len(todo) && calls < maxCalls; i, calls = i+BatchSize, calls+1 {
			batch := todo[i:min(i+BatchSize, len(todo))]
			got, err := p.AI.ScoreBatch(ctx, batch)
			if err != nil {
				// A spent budget is the plan working, not a fault: it is
				// logged, but does not mark the run partial.
				if errors.Is(err, ErrBudgetExhausted) {
					p.logger().Info("news: AI budget used up, using rules", "error", err)
				} else {
					p.logger().Warn("news: AI scoring failed, using rules", "error", err)
					report.Errors = append(report.Errors, "ai: "+err.Error())
				}
				break
			}
			for k, v := range got {
				out[k] = v
			}
		}
		if wa, ok := p.AI.(*WorkersAI); ok && wa.Budget != nil {
			after, _ := wa.Budget.Remaining(ctx)
			report.AINeurons = max(0, before-after)
		}
	}

	for _, c := range todo {
		if _, ok := out[c.Key]; !ok {
			out[c.Key] = RuleScore(c, now)
		}
	}
	return out
}

func isSupportingKey(k string) bool { return k == "newsdata" || len(k) > 9 && k[:9] == "newsdata:" }

func uniq(keys []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range keys {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}
