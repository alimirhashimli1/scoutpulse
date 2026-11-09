-- Football news: headlines collected from licensed feeds, grouped into
-- stories, scored for importance, and the best of them published.
--
-- Only what may be shown is stored: the original headline, a short original
-- summary, the source, the time and a link back. No article text and no
-- images, ever -- the feeds' descriptions are read in memory to write the
-- summary and then dropped.
--
-- This is a rolling window, not an archive. Each run keeps the newest 100
-- published stories (pinned ones aside) and deletes the rest with their
-- items, deletes stories that were never published once they are older than
-- the ingest window can reach, and trims the run log and usage counters.

CREATE TABLE IF NOT EXISTS news_stories (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The headline and link of the story's lead item, as the outlet wrote it.
    headline           TEXT NOT NULL,
    url                TEXT NOT NULL,
    source_name        TEXT NOT NULL,
    attribution        TEXT NOT NULL DEFAULT '',

    -- Written by the scorer, at most 40 words. Empty when the rule scorer
    -- scored it: that scorer cannot write original prose, and the feed's own
    -- wording is not ours to republish.
    summary            TEXT NOT NULL DEFAULT '',
    category           TEXT NOT NULL,
    score              SMALLINT NOT NULL,
    reason             TEXT NOT NULL DEFAULT '',
    scorer             TEXT NOT NULL,

    -- How many distinct outlets carried the story, and how many there were
    -- when it was last scored -- the difference is what triggers a rescore.
    source_count       INT NOT NULL DEFAULT 1,
    scored_sources     INT NOT NULL DEFAULT 0,

    -- When the earliest item about it appeared at its source.
    first_seen_at      TIMESTAMPTZ NOT NULL,
    -- When ScoutPulse published it. NULL while it is only a candidate.
    published_at       TIMESTAMPTZ,

    hidden             BOOLEAN NOT NULL DEFAULT FALSE,
    pinned             BOOLEAN NOT NULL DEFAULT FALSE,

    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT news_stories_score_range CHECK (score BETWEEN 1 AND 10),
    CONSTRAINT news_stories_category_valid
        CHECK (category IN ('transfers', 'injuries', 'results', 'managers', 'young_talents', 'other')),
    CONSTRAINT news_stories_scorer_valid CHECK (scorer IN ('ai', 'rules'))
);

-- The public feed: published, visible, pinned first, newest first.
CREATE INDEX IF NOT EXISTS idx_news_stories_feed
    ON news_stories (pinned DESC, published_at DESC, id)
    WHERE published_at IS NOT NULL AND NOT hidden;
-- Candidates and pruning.
CREATE INDEX IF NOT EXISTS idx_news_stories_seen ON news_stories (first_seen_at DESC);

CREATE TABLE IF NOT EXISTS news_items (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    story_id       UUID NOT NULL REFERENCES news_stories (id) ON DELETE CASCADE,
    source_key     TEXT NOT NULL,
    title          TEXT NOT NULL,
    url            TEXT NOT NULL,
    -- Scheme and host lowercased, tracking parameters and fragment removed.
    -- Unique, which is what makes a repeated run insert nothing.
    canonical_url  TEXT NOT NULL,
    published_at   TIMESTAMPTZ NOT NULL,
    fetched_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT news_items_canonical_unique UNIQUE (canonical_url)
);

CREATE INDEX IF NOT EXISTS idx_news_items_story ON news_items (story_id);

-- One row per pipeline run: what was fetched, what failed, what it cost.
CREATE TABLE IF NOT EXISTS news_runs (
    id           BIGSERIAL PRIMARY KEY,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at  TIMESTAMPTZ,
    status       TEXT NOT NULL DEFAULT 'running',
    fetched      INT NOT NULL DEFAULT 0,
    new_items    INT NOT NULL DEFAULT 0,
    scored       INT NOT NULL DEFAULT 0,
    published    INT NOT NULL DEFAULT 0,
    pruned       INT NOT NULL DEFAULT 0,
    ai_neurons   INT NOT NULL DEFAULT 0,
    errors       JSONB NOT NULL DEFAULT '[]',

    CONSTRAINT news_runs_status_valid CHECK (status IN ('running', 'ok', 'partial', 'failed'))
);

-- Daily counters behind the budgets: Workers AI Neurons and NewsData credits.
-- The day is UTC, matching when both providers reset their allowances.
CREATE TABLE IF NOT EXISTS news_usage (
    day     DATE NOT NULL,
    meter   TEXT NOT NULL,
    amount  INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (day, meter)
);

-- At most one run in flight, across every instance of the service. Enforced
-- by the database rather than an advisory lock, because a transaction-mode
-- connection pooler (Supabase's) does not keep session locks. A run that died
-- without finishing is marked failed by the next one after 20 minutes, which
-- is well past a run's own hard timeout.
CREATE UNIQUE INDEX IF NOT EXISTS news_runs_one_running
    ON news_runs ((TRUE)) WHERE status = 'running';
