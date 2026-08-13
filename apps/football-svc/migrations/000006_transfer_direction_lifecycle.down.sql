COMMENT ON TABLE transfers IS NULL;

-- Restored NOT VALID on purpose.
--
-- By the time this runs, rows may exist with both clubs nulled by a club
-- deletion -- which is exactly the situation the up migration exists to
-- permit. A plain ADD CONSTRAINT would fail against them, and the alternative,
-- deleting those rows to make it pass, would destroy history to satisfy a
-- constraint being reinstated only for symmetry.
--
-- NOT VALID applies the rule to new and updated rows while leaving existing
-- ones alone, so the rollback succeeds whatever the table contains. Run
-- VALIDATE CONSTRAINT by hand if you genuinely want it enforced retroactively;
-- it will fail if any such rows are present, which is the correct answer.
ALTER TABLE transfers DROP CONSTRAINT IF EXISTS transfers_has_direction;
ALTER TABLE transfers ADD CONSTRAINT transfers_has_direction
    CHECK (from_team_id IS NOT NULL OR to_team_id IS NOT NULL) NOT VALID;
