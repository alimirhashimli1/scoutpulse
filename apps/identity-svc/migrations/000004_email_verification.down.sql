DROP INDEX IF EXISTS idx_email_verification_user;
DROP INDEX IF EXISTS idx_email_verification_expiry;
DROP TABLE IF EXISTS email_verification_tokens;

ALTER TABLE users DROP COLUMN IF EXISTS email_verified;
