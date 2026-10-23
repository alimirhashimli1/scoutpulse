-- Articles: football analysis written by editors and administrators.
--
-- The body is a structured document (the editor's JSON, not HTML), so the
-- browser renders it through a fixed set of known blocks and no markup a
-- writer pastes can ever reach a reader's page as markup. Like a tactics
-- board, it is JSONB in a database this service already has.
--
-- An article moves through a small review workflow:
--
--   draft ──submit──▶ in_review ──publish──▶ published
--     ▲                  │  ▲                    │
--     │         request changes                  │
--     │                  ▼  │ submit             │
--     └──withdraw── changes_requested  ◀──withdraw┘ (back to draft)
--
-- An editor's article needs an administrator to publish it; an
-- administrator's own goes straight to published. "Scheduled" is not a state
-- of its own: it is a published article whose publish_at is still ahead, so
-- it goes live by the clock with nothing to run at that moment.

CREATE TABLE IF NOT EXISTS articles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The public address: words from the title, then a random suffix so two
    -- articles with the same headline never collide. Fixed from the moment an
    -- article is first published, so a link someone shared keeps working
    -- through every later edit.
    slug            TEXT NOT NULL,

    -- Accounts live in identity-svc's database, which no foreign key here can
    -- reach -- the same situation as tactic_boards. The name is captured from
    -- the token at write time because this service cannot resolve an id.
    author_id       UUID NOT NULL,
    author_name     TEXT NOT NULL DEFAULT '',

    -- A draft may be untitled; submitting needs a title (checked in the
    -- service, which can say so in words).
    title           TEXT NOT NULL DEFAULT '',
    subtitle        TEXT NOT NULL DEFAULT '',
    cover_url       TEXT NOT NULL DEFAULT '',
    cover_caption   TEXT NOT NULL DEFAULT '',

    body            JSONB NOT NULL DEFAULT '{"type":"doc","content":[]}',
    -- Derived from the body on every write, so lists and link previews can
    -- show them without reading or walking the document.
    excerpt         TEXT NOT NULL DEFAULT '',
    reading_minutes INT  NOT NULL DEFAULT 1,

    tags            TEXT[] NOT NULL DEFAULT '{}',

    status          TEXT NOT NULL DEFAULT 'draft',
    -- When a published article goes live. For an article in review it is the
    -- time its writer asked for, which the reviewing administrator can keep
    -- or change.
    publish_at      TIMESTAMPTZ,
    first_published_at TIMESTAMPTZ,
    submitted_at    TIMESTAMPTZ,

    -- The last review: who published it or sent it back, and what they said.
    reviewer_id     UUID,
    reviewer_name   TEXT NOT NULL DEFAULT '',
    reviewed_at     TIMESTAMPTZ,
    review_note     TEXT NOT NULL DEFAULT '',

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT articles_slug_unique UNIQUE (slug),
    CONSTRAINT articles_status_valid
        CHECK (status IN ('draft', 'in_review', 'changes_requested', 'published')),
    CONSTRAINT articles_published_has_time
        CHECK (status <> 'published' OR publish_at IS NOT NULL),
    CONSTRAINT articles_body_is_object CHECK (jsonb_typeof(body) = 'object')
);

-- The public list: live articles, newest first. Partial, because only
-- published rows are ever read this way.
CREATE INDEX IF NOT EXISTS idx_articles_live
    ON articles (publish_at DESC, id) WHERE status = 'published';
-- A writer's own desk, and account cleanup.
CREATE INDEX IF NOT EXISTS idx_articles_author ON articles (author_id, updated_at DESC);
-- The review queue.
CREATE INDEX IF NOT EXISTS idx_articles_status ON articles (status, updated_at DESC);
-- ?tag= on the public list.
CREATE INDEX IF NOT EXISTS idx_articles_tags ON articles USING GIN (tags);
