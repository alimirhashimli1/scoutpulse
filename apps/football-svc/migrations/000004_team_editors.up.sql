-- Editor team grants.
--
-- These used to be baked into the JWT as managed_team_ids, which meant a grant
-- took effect only after the editor logged in again, and a revocation could not
-- take effect at all until the token expired -- up to 24 hours of access after
-- it was withdrawn. Resolving grants per request fixes both, and stops the
-- token growing without bound as an editor accumulates clubs.
--
-- "Which clubs may this user edit" is football-domain data, so it lives in the
-- football service's database. identity-svc keeps owning who a user is and
-- their global role.
CREATE TABLE IF NOT EXISTS team_editors (
    -- No foreign key to a users table: users live in another service's
    -- database. Referential integrity across that boundary is the identity
    -- service's job, not a constraint this database can express.
    user_id    UUID NOT NULL,
    team_id    UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    granted_by UUID,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, team_id)
);

-- The hot path is "does this user manage this club", served by the primary
-- key. This index serves the reverse question -- who manages a given club.
CREATE INDEX IF NOT EXISTS idx_team_editors_team ON team_editors (team_id);
