-- Resolve a contradiction between two rules migration 000003 introduced.
--
-- Rule one, on the foreign keys:
--     from_team_id UUID REFERENCES teams(id) ON DELETE SET NULL
--     to_team_id   UUID REFERENCES teams(id) ON DELETE SET NULL
--   "A club may be deleted without erasing the transfer that references it."
--
-- Rule two, on the same table:
--     CONSTRAINT transfers_has_direction
--         CHECK (from_team_id IS NOT NULL OR to_team_id IS NOT NULL)
--   "A transfer must move a player from somewhere or to somewhere."
--
-- Both are reasonable; together they are impossible. A transfer with exactly
-- one club named -- an arrival from nowhere, or a release to nowhere -- loses
-- its only non-null reference when that club is deleted, and the CHECK then
-- refuses the delete.
--
-- This became reachable when creating a player started writing an opening
-- transfer (from_team_id NULL, to_team_id = the club joined). Every club with
-- players now has such a row, so DELETE /api/v1/teams/{id} failed with a
-- constraint violation surfaced as 400.
--
-- The CHECK is dropped rather than the SET NULL behaviour, because the two
-- rules are enforced at different times and only one of them belongs in the
-- database:
--
--   * "a transfer needs a direction" is a rule about *creating* one. The
--     service layer enforces it in validateTransfer, which rejects both-nil
--     with a clear message before any SQL runs.
--
--   * "history survives a club being deleted" is a rule about the row's whole
--     lifetime, and a CHECK cannot tell an insert from a cascade.
--
-- A transfer that has lost both clubs is degraded, not corrupt: the player,
-- the date, the type and the fee all survive, which is what makes the history
-- still worth keeping.

ALTER TABLE transfers DROP CONSTRAINT IF EXISTS transfers_has_direction;

-- The rule still holds for new rows, enforced where it can distinguish an
-- insert from a club deletion. Documented here so the absence of the CHECK
-- does not read as an oversight.
COMMENT ON TABLE transfers IS
    'Every move a player has made. A row must be created with at least one of '
    'from_team_id or to_team_id set -- enforced by service.validateTransfer, '
    'not by a CHECK, because ON DELETE SET NULL may later null both when the '
    'clubs are deleted. See migration 000006.';
