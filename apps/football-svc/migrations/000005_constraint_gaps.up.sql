-- Constraints the schema was missing, where a rule existed in prose or in the
-- service layer but nothing in the database enforced it.
--
-- Every other enumerated column here -- transfer_type, competition_type,
-- preferred_foot -- carries a CHECK, and the service layer carries a matching
-- Valid* helper. coach_spells.role and the seasons date ranges had neither.

-- ------------------------------------------------------- coach spell roles

-- role accepted any string, so "asdf" was a coaching role. Existing rows are
-- normalised to head_coach before the constraint goes on, since the column has
-- been writable without validation and may hold anything.
UPDATE coach_spells
   SET role = 'head_coach'
 WHERE role IS NULL
    OR role NOT IN ('head_coach', 'assistant_coach', 'interim_coach',
                    'caretaker', 'director_of_football', 'youth_coach');

ALTER TABLE coach_spells DROP CONSTRAINT IF EXISTS coach_spells_role_valid;
ALTER TABLE coach_spells ADD CONSTRAINT coach_spells_role_valid
    CHECK (role IN ('head_coach', 'assistant_coach', 'interim_coach',
                    'caretaker', 'director_of_football', 'youth_coach'));

-- ---------------------------------------------------------- season overlap

-- Overlapping seasons make "which season contains this date" ambiguous, and
-- the query resolving it takes the first by sort order -- so a transfer would
-- be filed against whichever season happened to sort first.
--
-- btree_gist supplies the operator class needed to mix the range overlap
-- operator with ordinary equality in one exclusion constraint.
CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE seasons DROP CONSTRAINT IF EXISTS seasons_no_overlap;
ALTER TABLE seasons ADD CONSTRAINT seasons_no_overlap
    EXCLUDE USING gist (daterange(start_date, end_date, '[]') WITH &&);
