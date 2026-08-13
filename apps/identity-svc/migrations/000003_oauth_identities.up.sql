-- Sign in with Google or Facebook.
--
-- Three things this has to get right, and the schema is where two of them are
-- enforced.

-- ------------------------------------------------------- passwordless users

-- An account created through a provider has no password, and never will unless
-- the owner sets one. password_hash was NOT NULL, which forced a placeholder
-- value -- and a placeholder in a password column is exactly the sort of thing
-- that later gets compared against.
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

-- ----------------------------------------------------------- the identities

-- One row per (provider, account-at-that-provider). A user may hold several:
-- Google and Facebook both linked to one ScoutPulse account is the normal
-- case, not an edge case.
CREATE TABLE IF NOT EXISTS user_identities (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         VARCHAR(32) NOT NULL,
    -- The provider's own immutable id for the account, NOT the email. Email
    -- addresses at a provider can be changed and reassigned; the subject id
    -- cannot, and it is what the account is actually keyed on.
    provider_user_id VARCHAR(255) NOT NULL,
    -- Recorded as it was at link time, for display and support. Never used to
    -- find the account -- see the note above.
    email            VARCHAR(255),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at    TIMESTAMPTZ,

    CONSTRAINT user_identities_provider_valid CHECK (provider IN ('google', 'facebook')),

    -- One provider account maps to exactly one user. Without this, two local
    -- accounts could both claim the same Google identity and which one you got
    -- would depend on row order.
    CONSTRAINT user_identities_provider_account_unique UNIQUE (provider, provider_user_id),

    -- And a user links each provider at most once.
    CONSTRAINT user_identities_user_provider_unique UNIQUE (user_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_user_identities_user ON user_identities (user_id);

-- --------------------------------------------------- one-time login handoff

-- The provider callback lands on the backend, but the tokens belong to the
-- browser app. Putting them in the redirect URL would write a refresh token
-- into browser history, server logs and any Referer header the next page
-- sends.
--
-- Instead the callback issues a short-lived single-use code, and the frontend
-- exchanges it for the normal token pair over POST -- the same shape it
-- already uses for password login.
CREATE TABLE IF NOT EXISTS oauth_login_codes (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Stored as a SHA-256 hash for the same reason refresh tokens are: reading
    -- this table must not yield a usable credential.
    code_hash  BYTEA NOT NULL UNIQUE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oauth_login_codes_expiry ON oauth_login_codes (expires_at);
