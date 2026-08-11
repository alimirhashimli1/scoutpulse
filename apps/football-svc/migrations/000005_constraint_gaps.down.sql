ALTER TABLE seasons DROP CONSTRAINT IF EXISTS seasons_no_overlap;

-- btree_gist is left installed. Dropping an extension another object may have
-- come to depend on is riskier than leaving it, and it is inert when unused.

ALTER TABLE coach_spells DROP CONSTRAINT IF EXISTS coach_spells_role_valid;
