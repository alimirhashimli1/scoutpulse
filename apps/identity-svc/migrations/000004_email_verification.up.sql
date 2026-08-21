-- Email verification for password registrations.
--
-- An address typed into a form is a claim, not a fact. Without checking it, an
-- account can be created against someone else's address -- which matters here
-- because an address is what links an external sign-in to an existing account.

ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT FALSE;

-- Accounts that already exist predate this rule. Leaving them unverified would
-- lock out every current user the moment login starts checking the flag, which
-- would be a regression dressed as a security improvement.
UPDATE users SET email_verified = TRUE WHERE created_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- SHA-256, for the same reason refresh tokens and sign-in codes are
    -- hashed: reading this table must not hand anyone a working link.
    token_hash  BYTEA NOT NULL UNIQUE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL,
    -- Set when the link is followed. Single use: a verification link sitting
    -- in an inbox is a credential, and inboxes get forwarded and breached.
    consumed_at TIMESTAMPTZ
);

-- Sweeping expired rows, and finding a user's outstanding tokens when a
-- resend has to invalidate them.
CREATE INDEX IF NOT EXISTS idx_email_verification_expiry ON email_verification_tokens (expires_at);
CREATE INDEX IF NOT EXISTS idx_email_verification_user ON email_verification_tokens (user_id);
