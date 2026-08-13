DROP INDEX IF EXISTS idx_oauth_login_codes_expiry;
DROP TABLE IF EXISTS oauth_login_codes;

DROP INDEX IF EXISTS idx_user_identities_user;
DROP TABLE IF EXISTS user_identities;

-- Restoring NOT NULL on password_hash would fail against any account created
-- through a provider, which has no password by design. Those rows are given a
-- value that cannot match a bcrypt comparison -- bcrypt rejects a malformed
-- hash outright, so it is unusable as a credential rather than being a weak
-- one.
UPDATE users SET password_hash = '!provider-account-no-password' WHERE password_hash IS NULL;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
