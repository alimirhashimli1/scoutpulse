-- Refresh tokens.
--
-- Access tokens are stateless: once signed, nothing can recall them, so the
-- only bound on a leaked one is its expiry. They are now short-lived (15
-- minutes) and long-lived access is carried by a refresh token instead, which
-- has a row here and can therefore be revoked immediately.
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- The SHA-256 of the token, never the token itself. Someone who reads
    -- this table must not come away with a set of working sessions.
    token_hash BYTEA NOT NULL UNIQUE,

    issued_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,

    -- Set when this token is rotated, forming a chain. Presenting a token
    -- that has already been replaced means it leaked, and the whole chain is
    -- then revoked.
    replaced_by UUID REFERENCES refresh_tokens(id) ON DELETE SET NULL,

    user_agent TEXT
);

-- The lookup on every refresh is by hash; the unique constraint serves it.
-- This index serves revoking every session a user holds.
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens (user_id);

-- Supports pruning expired rows.
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires ON refresh_tokens (expires_at);

-- Editor club grants have moved to the football service, which owns the clubs
-- they refer to. Keeping them here meant they had to be copied into every
-- token, which froze an editor's permissions for the token's lifetime: a new
-- grant needed a fresh login and a revocation could not take effect at all.
ALTER TABLE users DROP COLUMN IF EXISTS managed_team_ids;

-- Track role changes, which are now an administrative action rather than
-- something a registering user can choose for themselves.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS role_updated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS role_updated_by UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_valid;
ALTER TABLE users ADD CONSTRAINT users_role_valid CHECK (role IN ('admin', 'editor', 'user'));
