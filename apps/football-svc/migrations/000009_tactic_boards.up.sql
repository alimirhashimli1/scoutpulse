-- Tactics boards: animated drills and match analysis drawn by members.
--
-- A board is stored as data -- where every player, cone and arrow is at each
-- step -- and the browser animates it. That is what keeps this cheap: a whole
-- animated drill is tens of kilobytes of JSON in a table this database already
-- has, where the same thing as video would be megabytes in object storage.

CREATE TABLE IF NOT EXISTS tactic_boards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The public address. Random rather than derived from id or title, because
    -- a board is unlisted: anyone holding the link can watch it, and nobody
    -- should be able to find the next one by counting.
    slug        TEXT NOT NULL,

    -- Accounts live in identity-svc's database, which no foreign key here can
    -- reach. The same situation as player_notes.
    owner_id    UUID NOT NULL,
    -- Denormalised from the token at write time, because this service cannot
    -- resolve a user id to a name. It is what the members' list groups by.
    owner_name  TEXT NOT NULL DEFAULT '',

    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',

    -- The board itself. Opaque to this service beyond being a JSON object of
    -- bounded size: its shape is the editor's concern, and versioned inside
    -- the document so an old board can still be read by a newer editor.
    content     JSONB NOT NULL,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT tactic_boards_slug_unique UNIQUE (slug),
    CONSTRAINT tactic_boards_title_not_empty CHECK (length(btrim(title)) > 0),
    CONSTRAINT tactic_boards_content_is_object CHECK (jsonb_typeof(content) = 'object')
);

-- The members' list reads every board grouped by author, newest first inside
-- each group.
CREATE INDEX IF NOT EXISTS idx_tactic_boards_owner_name
    ON tactic_boards (lower(owner_name), owner_id, updated_at DESC);
-- "My boards", and account cleanup, both go by owner.
CREATE INDEX IF NOT EXISTS idx_tactic_boards_owner ON tactic_boards (owner_id, updated_at DESC);
