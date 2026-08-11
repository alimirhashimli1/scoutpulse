ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_valid;
ALTER TABLE users
    DROP COLUMN IF EXISTS role_updated_by,
    DROP COLUMN IF EXISTS role_updated_at;

ALTER TABLE users ADD COLUMN IF NOT EXISTS managed_team_ids UUID[] DEFAULT '{}';

DROP INDEX IF EXISTS idx_refresh_tokens_expires;
DROP INDEX IF EXISTS idx_refresh_tokens_user;
DROP TABLE IF EXISTS refresh_tokens;
