// Package importer loads a public football dataset into the football
// database: competitions, clubs, squads, transfer histories, market values
// and season statistics.
//
// It writes to Postgres directly rather than through the API. The API is the
// right door for an editor changing one player; it is the wrong one for seven
// thousand players and a hundred thousand history rows, which over HTTP would
// mean hours of requests, the per-account rate limit, and no way to make the
// whole load succeed or fail as one.
//
// Three rules shape it:
//
//   - **Idempotent.** Every row it creates is recorded in external_refs
//     against the source's own id, so a second run updates what the first
//     created instead of adding copies. Re-running after the source refreshes
//     is how the data is kept current.
//   - **Hand-entered data wins where it is a judgement.** A player already in
//     the database is matched rather than duplicated, and keeps their notes,
//     scouting percentages, strengths, weaknesses, secondary positions,
//     squad number and nationalities. Facts the source is authoritative on --
//     club, date of birth, height, contract, market value -- are corrected.
//   - **All or nothing.** One transaction. A load that fails halfway leaves
//     the database exactly as it was.
package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/scoutpulse/football-svc/internal/domain"
)

// Source is the external_refs.source of everything this package writes.
const Source = "transfermarkt"

// Report says what an import did, and what it could not reconcile.
type Report struct {
	LeaguesCreated, LeaguesMatched int
	SeasonsCreated                 int
	ClubsCreated, ClubsMatched     int
	PlayersCreated, PlayersMatched int
	TransfersCreated               int
	TransfersUpdated               int
	TransfersAdopted               int
	OpeningsRemoved                int
	Valuations                     int
	StatLines                      int
	Entries                        int

	// Clubs already in the database, in a league this import covers, that
	// the source does not have in that league now. Most are relegated clubs
	// or a misfiled league; either way they are left alone for a person to
	// decide about.
	ClubsNotInSource []string
	// Players already at a club this import covers whom the source does not
	// have in that squad. Usually they have moved on; they are left alone.
	PlayersNotInSource []string
}

// Import merges d into the database. With dryRun it does all the work and
// rolls it back, which is how to see the report before committing to it.
func Import(ctx context.Context, db *sqlx.DB, d *Dataset, scope Scope, dryRun bool, log *slog.Logger) (*Report, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Some date columns are timestamptz (contract_until), and a date cast to
	// one lands on midnight in the session's zone. UTC is what the API writes.
	if _, err := tx.ExecContext(ctx, `SET LOCAL TIME ZONE 'UTC'`); err != nil {
		return nil, err
	}

	var stats, refsTable sql.NullString
	if err := tx.QueryRowContext(ctx,
		`SELECT to_regclass('player_season_stats')::text, to_regclass('external_refs')::text`,
	).Scan(&stats, &refsTable); err != nil {
		return nil, err
	}
	if !stats.Valid || !refsTable.Valid {
		return nil, errors.New("this database is missing migration 000011 (player_season_stats, external_refs): apply it first")
	}

	m := &merge{ctx: ctx, tx: tx, d: d, scope: scope, log: log, report: &Report{}}
	steps := []struct {
		name string
		fn   func() error
	}{
		{"references", m.loadRefs},
		{"competitions", m.importCompetitions},
		{"seasons", m.importSeasons},
		{"clubs", m.importClubs},
		{"players", m.importPlayers},
		{"market values", m.importValuations},
		{"transfers", m.importTransfers},
		{"season stats", m.importStats},
		{"competition entries", m.importEntries},
		{"saving references", m.saveRefs},
	}
	for _, s := range steps {
		started := time.Now()
		if err := s.fn(); err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
		log.Info("step done", "step", s.name, "took", time.Since(started).Round(time.Millisecond))
	}

	if dryRun {
		log.Info("dry run: rolling back")
		return m.report, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return m.report, nil
}

type season struct {
	ID    string `db:"id"`
	Start string `db:"start_date"`
	End   string `db:"end_date"`
}

type merge struct {
	ctx    context.Context
	tx     *sqlx.Tx
	d      *Dataset
	scope  Scope
	log    *slog.Logger
	report *Report

	refs    map[string]map[string]string // kind -> external id -> local id
	newRefs []ref

	comps   map[string]string // competition id -> league id
	seasons []season          // every season, after creation
	clubs   map[string]string // club id -> team id
	players map[string]string // player id -> local id
	// matched are local players that existed before this run began.
	matched map[string]bool
}

type ref struct{ kind, external, local string }

func (m *merge) ref(kind, external string) (string, bool) {
	id, ok := m.refs[kind][external]
	return id, ok
}

func (m *merge) remember(kind, external, local string) {
	if m.refs[kind] == nil {
		m.refs[kind] = map[string]string{}
	}
	if m.refs[kind][external] == local {
		return
	}
	m.refs[kind][external] = local
	m.newRefs = append(m.newRefs, ref{kind, external, local})
}

func (m *merge) loadRefs() error {
	m.refs = map[string]map[string]string{}
	rows, err := m.tx.QueryContext(m.ctx,
		`SELECT kind, external_id, local_id::text FROM external_refs WHERE source = $1`, Source)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kind, ext, local string
		if err := rows.Scan(&kind, &ext, &local); err != nil {
			return err
		}
		if m.refs[kind] == nil {
			m.refs[kind] = map[string]string{}
		}
		m.refs[kind][ext] = local
	}
	return rows.Err()
}

func (m *merge) saveRefs() error {
	n := len(m.newRefs)
	return inChunks(n, func(lo, hi int) error {
		kinds, exts, locals := make([]string, 0, hi-lo), make([]string, 0, hi-lo), make([]string, 0, hi-lo)
		for _, r := range m.newRefs[lo:hi] {
			kinds, exts, locals = append(kinds, r.kind), append(exts, r.external), append(locals, r.local)
		}
		_, err := m.tx.ExecContext(m.ctx, `
			INSERT INTO external_refs (source, kind, external_id, local_id)
			SELECT $1, k, e, l::uuid FROM unnest($2::text[], $3::text[], $4::text[]) AS t(k, e, l)
			ON CONFLICT (source, kind, external_id)
			DO UPDATE SET local_id = EXCLUDED.local_id, imported_at = NOW()`,
			Source, pq.Array(kinds), pq.Array(exts), pq.Array(locals))
		return err
	})
}

// ---------------------------------------------------------- competitions

func compactKey(s string) string { return strings.ReplaceAll(fold(s), " ", "") }

func (m *merge) importCompetitions() error {
	type league struct{ ID, Name string }
	var existing []league
	if err := m.tx.SelectContext(m.ctx, &existing, `SELECT id::text AS id, name FROM leagues`); err != nil {
		return err
	}
	byID := map[string]bool{}
	byName := map[string]string{}
	for _, l := range existing {
		byID[l.ID] = true
		byName[compactKey(l.Name)] = l.ID
	}

	m.comps = map[string]string{}
	ids := make([]string, 0)
	for id := range competitionsFor(m.scope.Leagues) {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, code := range ids {
		c := Catalog[code]
		local, ok := m.ref("league", code)
		if ok && !byID[local] {
			ok = false
		}
		if !ok {
			local, ok = byName[compactKey(c.Name)]
		}
		var tier *int
		if c.Type == domain.CompetitionLeague {
			one := 1
			tier = &one
		}
		if ok {
			m.report.LeaguesMatched++
			if _, err := m.tx.ExecContext(m.ctx,
				`UPDATE leagues SET competition_type = $1, tier = COALESCE(tier, $2) WHERE id = $3`,
				string(c.Type), tier, local); err != nil {
				return err
			}
		} else {
			m.report.LeaguesCreated++
			if err := m.tx.QueryRowContext(m.ctx,
				`INSERT INTO leagues (name, country, tier, competition_type) VALUES ($1, $2, $3, $4) RETURNING id::text`,
				c.Name, c.Country, tier, string(c.Type)).Scan(&local); err != nil {
				return err
			}
		}
		m.comps[code] = local
		m.remember("league", code, local)
	}
	return nil
}

// --------------------------------------------------------------- seasons

func (m *merge) loadSeasons() error {
	m.seasons = nil
	return m.tx.SelectContext(m.ctx, &m.seasons,
		`SELECT id::text AS id, start_date::text AS start_date, end_date::text AS end_date FROM seasons ORDER BY start_date`)
}

// seasonOn is the season containing date, or nil.
func (m *merge) seasonOn(date string) *string {
	for i := range m.seasons {
		if m.seasons[i].Start <= date && date <= m.seasons[i].End {
			return &m.seasons[i].ID
		}
	}
	return nil
}

func (m *merge) importSeasons() error {
	if err := m.loadSeasons(); err != nil {
		return err
	}
	for year := m.scope.FirstSeason; year <= m.d.LatestSeason; year++ {
		// The middle of August is inside every season however its edges are
		// drawn, so it finds an existing one even with different dates.
		if m.seasonOn(fmt.Sprintf("%d-08-15", year)) != nil {
			continue
		}
		start, end := fmt.Sprintf("%d-07-01", year), fmt.Sprintf("%d-06-30", year+1)
		// Shrink to fit between neighbours rather than fail on the overlap
		// constraint, if someone drew a season with unusual edges.
		for _, s := range m.seasons {
			if s.End >= start && s.End < fmt.Sprintf("%d-08-15", year) {
				start = dayAfter(s.End)
			}
			if s.Start <= end && s.Start > fmt.Sprintf("%d-08-15", year) {
				end = dayBefore(s.Start)
			}
		}
		if _, err := m.tx.ExecContext(m.ctx,
			`INSERT INTO seasons (label, start_date, end_date) VALUES ($1, $2, $3)
			 ON CONFLICT (label) DO NOTHING`, seasonLabel(year), start, end); err != nil {
			return err
		}
		m.report.SeasonsCreated++
		if err := m.loadSeasons(); err != nil {
			return err
		}
	}
	return nil
}

func dayAfter(date string) string {
	t, _ := time.Parse("2006-01-02", date)
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

func dayBefore(date string) string {
	t, _ := time.Parse("2006-01-02", date)
	return t.AddDate(0, 0, -1).Format("2006-01-02")
}

// seasonID is the local id of the season starting in year.
func (m *merge) seasonID(year int) *string {
	return m.seasonOn(fmt.Sprintf("%d-08-15", year))
}

// ----------------------------------------------------------------- clubs

type localTeam struct {
	ID        string         `db:"id"`
	Name      string         `db:"name"`
	ShortName sql.NullString `db:"short_name"`
	LeagueID  sql.NullString `db:"league_id"`
	Country   sql.NullString `db:"country"`
}

func (m *merge) importClubs() error {
	var existing []localTeam
	if err := m.tx.SelectContext(m.ctx, &existing,
		`SELECT id::text AS id, name, short_name, league_id::text AS league_id, country FROM teams`); err != nil {
		return err
	}
	byID := map[string]localTeam{}
	byKey := map[string][]string{}
	for _, t := range existing {
		byID[t.ID] = t
		keys := map[string]bool{clubKey(t.Name): true}
		if t.ShortName.Valid && t.ShortName.String != "" {
			keys[clubKey(t.ShortName.String)] = true
		}
		for k := range keys {
			byKey[k] = append(byKey[k], t.ID)
		}
	}

	ids := make([]string, 0, len(m.d.Clubs))
	for id := range m.d.Clubs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	m.clubs = map[string]string{}
	taken := map[string]bool{}
	var newIDs, newNames, newCountries, newStadiums []string
	var newLeagues []sql.NullString
	var updIDs, updStadiums, updCountries []string
	var updLeagues []sql.NullString

	for _, cid := range ids {
		c := m.d.Clubs[cid]
		country := Catalog[c.League].Country
		var league sql.NullString
		if c.LastSeason == m.d.LatestSeason {
			league = sql.NullString{String: m.comps[c.League], Valid: true}
		}

		local, ok := m.ref("team", cid)
		if ok {
			_, ok = byID[local]
		}
		if !ok {
			local, ok = matchClub(c, country, byKey, byID, taken)
		}

		if ok {
			m.report.ClubsMatched++
			updIDs, updStadiums, updCountries = append(updIDs, local), append(updStadiums, c.Stadium), append(updCountries, country)
			updLeagues = append(updLeagues, league)
		} else {
			m.report.ClubsCreated++
			local = uuid.NewString()
			newIDs, newNames, newCountries, newStadiums = append(newIDs, local), append(newNames, c.Name), append(newCountries, country), append(newStadiums, c.Stadium)
			newLeagues = append(newLeagues, league)
		}
		taken[local] = true
		m.clubs[cid] = local
		m.remember("team", cid, local)
	}

	if len(newIDs) > 0 {
		if _, err := m.tx.ExecContext(m.ctx, `
			INSERT INTO teams (id, name, league_id, country, stadium)
			SELECT i::uuid, n, l::uuid, c, NULLIF(s, '')
			  FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[]) AS t(i, n, l, c, s)`,
			pq.Array(newIDs), pq.Array(newNames), pq.Array(newLeagues), pq.Array(newCountries), pq.Array(newStadiums)); err != nil {
			return err
		}
	}
	// A matched club keeps its name, which someone chose. Its league is set
	// only when the source has it in one now: a relegated club keeps whatever
	// second-tier league it was filed under here.
	if len(updIDs) > 0 {
		if _, err := m.tx.ExecContext(m.ctx, `
			UPDATE teams t
			   SET league_id = COALESCE(u.l::uuid, t.league_id),
			       stadium   = COALESCE(t.stadium, NULLIF(u.s, '')),
			       country   = COALESCE(t.country, u.c)
			  FROM unnest($1::text[], $2::text[], $3::text[], $4::text[]) AS u(i, l, s, c)
			 WHERE t.id = u.i::uuid`,
			pq.Array(updIDs), pq.Array(updLeagues), pq.Array(updStadiums), pq.Array(updCountries)); err != nil {
			return err
		}
	}

	covered := map[string]bool{}
	for code, id := range m.comps {
		if Catalog[code].Type == domain.CompetitionLeague {
			covered[id] = true
		}
	}
	for _, t := range existing {
		if !taken[t.ID] && t.LeagueID.Valid && covered[t.LeagueID.String] {
			m.report.ClubsNotInSource = append(m.report.ClubsNotInSource, t.Name)
		}
	}
	sort.Strings(m.report.ClubsNotInSource)
	return nil
}

// matchClub finds the existing club a source club is, by name.
//
// First an exact match on the folded name without its legal form, then --
// for "Brighton" against "Brighton & Hove Albion" -- a club in the same
// country whose name starts with the other's. Either must be unambiguous.
func matchClub(c Club, country string, byKey map[string][]string, byID map[string]localTeam, taken map[string]bool) (string, bool) {
	free := func(ids []string) []string {
		var out []string
		for _, id := range ids {
			if !taken[id] {
				out = append(out, id)
			}
		}
		return out
	}

	key := clubKey(c.Name)
	if hits := free(byKey[key]); len(hits) == 1 {
		return hits[0], true
	}

	var hits []string
	for k, ids := range byKey {
		if k == "" || (!strings.HasPrefix(key, k+" ") && !strings.HasPrefix(k, key+" ")) {
			continue
		}
		for _, id := range free(ids) {
			if t := byID[id]; !t.Country.Valid || fold(t.Country.String) == fold(country) {
				hits = append(hits, id)
			}
		}
	}
	if len(hits) == 1 {
		return hits[0], true
	}
	return "", false
}

// --------------------------------------------------------------- players

type localPlayer struct {
	ID            string         `db:"id"`
	Name          string         `db:"name"`
	DateOfBirth   sql.NullString `db:"date_of_birth"`
	TeamID        sql.NullString `db:"team_id"`
	ContractStart sql.NullString `db:"contract_start"`
	Created       string         `db:"created"`
}

func (m *merge) importPlayers() error {
	var existing []localPlayer
	if err := m.tx.SelectContext(m.ctx, &existing, `
		SELECT id::text AS id, name, date_of_birth::text AS date_of_birth, team_id::text AS team_id,
		       contract_start::text AS contract_start, created_at::date::text AS created
		  FROM players`); err != nil {
		return err
	}
	byID := map[string]localPlayer{}
	byNameDOB := map[string][]string{}
	byNameTeam := map[string][]string{}
	byName := map[string][]string{}
	for _, p := range existing {
		byID[p.ID] = p
		n := fold(p.Name)
		byName[n] = append(byName[n], p.ID)
		if p.DateOfBirth.Valid {
			byNameDOB[n+"|"+p.DateOfBirth.String] = append(byNameDOB[n+"|"+p.DateOfBirth.String], p.ID)
		}
		if p.TeamID.Valid {
			byNameTeam[n+"|"+p.TeamID.String] = append(byNameTeam[n+"|"+p.TeamID.String], p.ID)
		}
	}
	sourceNames := map[string]int{}
	for _, p := range m.d.Players {
		sourceNames[fold(p.Name)]++
	}

	latestValue := map[string]int64{}
	latestDate := map[string]string{}
	for _, v := range m.d.Valuations {
		if v.Date > latestDate[v.PlayerID] {
			latestDate[v.PlayerID], latestValue[v.PlayerID] = v.Date, v.EUR
		}
	}

	m.players = map[string]string{}
	m.matched = map[string]bool{}
	taken := map[string]bool{}
	cols := playerColumns{}

	for _, p := range m.d.Players {
		team := m.clubs[p.ClubID]
		n := fold(p.Name)

		local, ok := m.ref("player", p.ID)
		if ok {
			_, ok = byID[local]
		}
		// A unique hit or none: two existing players who both fit is a
		// question for a person, and creating a third is safer than guessing.
		pick := func(ids []string) {
			var free []string
			for _, id := range ids {
				if !taken[id] {
					free = append(free, id)
				}
			}
			if len(free) == 1 {
				local, ok = free[0], true
			}
		}
		if !ok && p.DateOfBirth != "" {
			pick(byNameDOB[n+"|"+p.DateOfBirth])
		}
		if !ok {
			pick(byNameTeam[n+"|"+team])
		}
		// A name alone, only when nobody else in the source shares it and the
		// existing player has no date of birth to contradict it.
		if !ok && sourceNames[n] == 1 && len(byName[n]) == 1 && !byID[byName[n][0]].DateOfBirth.Valid {
			pick(byName[n])
		}

		if ok {
			m.report.PlayersMatched++
			m.matched[local] = true
		} else {
			m.report.PlayersCreated++
			local = uuid.NewString()
		}
		taken[local] = true
		m.players[p.ID] = local
		m.remember("player", p.ID, local)

		value := p.MarketValueEUR
		if value == 0 {
			value = latestValue[p.ID]
		}
		cols.add(local, team, p, value*100)
	}

	if err := inChunks(len(cols.ids), func(lo, hi int) error {
		_, err := m.tx.ExecContext(m.ctx, upsertPlayers, cols.args(lo, hi)...)
		return err
	}); err != nil {
		return err
	}

	// Squad members here the source does not list at that club.
	for _, p := range existing {
		if taken[p.ID] || !p.TeamID.Valid {
			continue
		}
		for _, team := range m.clubs {
			if team == p.TeamID.String {
				m.report.PlayersNotInSource = append(m.report.PlayersNotInSource, p.Name)
				break
			}
		}
	}
	sort.Strings(m.report.PlayersNotInSource)
	return nil
}

// upsertPlayers inserts new players and corrects matched ones.
//
// On a match the name stays as entered, nationalities stay if any were
// entered (the source records one citizenship; a person may have entered
// two), and squad number, scouting fields and notes are not touched. The main
// position is dropped from the secondary list if it is now there, which the
// API would otherwise refuse on the next edit.
const upsertPlayers = `
	INSERT INTO players (id, team_id, name, first_name, last_name, date_of_birth, nationalities,
	                     height_cm, preferred_foot, agent, position, contract_until,
	                     market_value_minor, currency, international_caps, international_goals)
	SELECT i::uuid, t::uuid, n, NULLIF(fn, ''), NULLIF(ln, ''), NULLIF(dob, '')::date,
	       CASE WHEN nat = '' THEN '{}'::text[] ELSE ARRAY[nat] END,
	       NULLIF(h, 0), NULLIF(foot, ''), NULLIF(ag, ''), pos, NULLIF(cu, '')::date,
	       v, 'EUR', caps, goals
	  FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[],
	              $8::int[], $9::text[], $10::text[], $11::text[], $12::text[], $13::bigint[],
	              $14::int[], $15::int[])
	       AS u(i, t, n, fn, ln, dob, nat, h, foot, ag, pos, cu, v, caps, goals)
	ON CONFLICT (id) DO UPDATE SET
	    team_id             = EXCLUDED.team_id,
	    first_name          = COALESCE(EXCLUDED.first_name, players.first_name),
	    last_name           = COALESCE(EXCLUDED.last_name, players.last_name),
	    date_of_birth       = COALESCE(EXCLUDED.date_of_birth, players.date_of_birth),
	    nationalities       = CASE WHEN cardinality(players.nationalities) > 0
	                               THEN players.nationalities ELSE EXCLUDED.nationalities END,
	    height_cm           = COALESCE(EXCLUDED.height_cm, players.height_cm),
	    preferred_foot      = COALESCE(EXCLUDED.preferred_foot, players.preferred_foot),
	    agent               = COALESCE(EXCLUDED.agent, players.agent),
	    position            = EXCLUDED.position,
	    secondary_positions = array_remove(players.secondary_positions, EXCLUDED.position),
	    contract_until      = COALESCE(EXCLUDED.contract_until, players.contract_until),
	    market_value_minor  = CASE WHEN EXCLUDED.market_value_minor > 0
	                               THEN EXCLUDED.market_value_minor ELSE players.market_value_minor END,
	    international_caps  = COALESCE(EXCLUDED.international_caps, players.international_caps),
	    international_goals = COALESCE(EXCLUDED.international_goals, players.international_goals)`

type playerColumns struct {
	ids, teams, names, first, last, dobs, nats, feet, agents, positions, contracts []string
	heights                                                                        []int64
	values                                                                         []int64
	caps, goals                                                                    []sql.NullInt64
}

func (c *playerColumns) add(id, team string, p Player, valueMinor int64) {
	position := p.SubPosition
	if position == "" || position == "Missing" {
		position = p.Position
	}
	if position == "" || position == "Missing" {
		position = "Unknown"
	}
	foot := strings.ToLower(p.Foot)
	if foot != "left" && foot != "right" && foot != "both" {
		foot = ""
	}
	c.ids = append(c.ids, id)
	c.teams = append(c.teams, team)
	c.names = append(c.names, p.Name)
	c.first = append(c.first, p.FirstName)
	c.last = append(c.last, p.LastName)
	c.dobs = append(c.dobs, p.DateOfBirth)
	c.nats = append(c.nats, p.Citizenship)
	c.heights = append(c.heights, int64(p.HeightCM))
	c.feet = append(c.feet, foot)
	c.agents = append(c.agents, p.Agent)
	c.positions = append(c.positions, position)
	c.contracts = append(c.contracts, p.ContractUntil)
	c.values = append(c.values, valueMinor)
	c.caps = append(c.caps, nullInt(p.InternationalCaps))
	c.goals = append(c.goals, nullInt(p.InternationalGoals))
}

func (c *playerColumns) args(lo, hi int) []any {
	return []any{
		pq.Array(c.ids[lo:hi]), pq.Array(c.teams[lo:hi]), pq.Array(c.names[lo:hi]),
		pq.Array(c.first[lo:hi]), pq.Array(c.last[lo:hi]), pq.Array(c.dobs[lo:hi]),
		pq.Array(c.nats[lo:hi]), pq.Array(c.heights[lo:hi]), pq.Array(c.feet[lo:hi]),
		pq.Array(c.agents[lo:hi]), pq.Array(c.positions[lo:hi]), pq.Array(c.contracts[lo:hi]),
		pq.Array(c.values[lo:hi]), pq.Array(c.caps[lo:hi]), pq.Array(c.goals[lo:hi]),
	}
}

func nullInt(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

// --------------------------------------------------------- market values

func (m *merge) importValuations() error {
	var ids, dates []string
	var values []int64
	for _, v := range m.d.Valuations {
		local, ok := m.players[v.PlayerID]
		if !ok || v.Date == "" {
			continue
		}
		ids, dates, values = append(ids, local), append(dates, v.Date), append(values, v.EUR*100)
	}
	m.report.Valuations = len(ids)
	return inChunks(len(ids), func(lo, hi int) error {
		_, err := m.tx.ExecContext(m.ctx, `
			INSERT INTO player_market_values (player_id, valued_on, value_minor, currency)
			SELECT p::uuid, d::date, v, 'EUR' FROM unnest($1::text[], $2::text[], $3::bigint[]) AS t(p, d, v)
			ON CONFLICT (player_id, valued_on) DO UPDATE SET value_minor = EXCLUDED.value_minor`,
			pq.Array(ids[lo:hi]), pq.Array(dates[lo:hi]), pq.Array(values[lo:hi]))
		return err
	})
}

// ------------------------------------------------------------- transfers

type localTransfer struct {
	ID       string         `db:"id"`
	PlayerID string         `db:"player_id"`
	From     sql.NullString `db:"from_team_id"`
	To       sql.NullString `db:"to_team_id"`
	Date     string         `db:"transfer_date"`
	Type     string         `db:"transfer_type"`
	Fee      sql.NullInt64  `db:"fee_minor"`
}

type transferColumns struct {
	ids, players, dates, types []string
	from, to, seasons          []sql.NullString
	fees                       []sql.NullInt64
}

func (c *transferColumns) add(id, player string, from, to, season *string, date string, kind domain.TransferType, fee *int64) {
	c.ids = append(c.ids, id)
	c.players = append(c.players, player)
	c.from = append(c.from, nullString(from))
	c.to = append(c.to, nullString(to))
	c.seasons = append(c.seasons, nullString(season))
	c.dates = append(c.dates, date)
	c.types = append(c.types, string(kind))
	if fee == nil {
		c.fees = append(c.fees, sql.NullInt64{})
	} else {
		c.fees = append(c.fees, sql.NullInt64{Int64: *fee, Valid: true})
	}
}

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func (m *merge) importTransfers() error {
	locals := make([]string, 0, len(m.players))
	for _, id := range m.players {
		locals = append(locals, id)
	}
	var existing []localTransfer
	if err := m.tx.SelectContext(m.ctx, &existing, `
		SELECT id::text AS id, player_id::text AS player_id, from_team_id::text AS from_team_id,
		       to_team_id::text AS to_team_id, transfer_date::text AS transfer_date, transfer_type, fee_minor
		  FROM transfers WHERE player_id = ANY($1::uuid[])`, pq.Array(locals)); err != nil {
		return err
	}
	byID := map[string]localTransfer{}
	byPlayer := map[string][]localTransfer{}
	refByLocal := map[string]bool{}
	for _, id := range m.refs["transfer"] {
		refByLocal[id] = true
	}
	for _, t := range existing {
		byID[t.ID] = t
		byPlayer[t.PlayerID] = append(byPlayer[t.PlayerID], t)
	}

	inserts, updates := transferColumns{}, transferColumns{}
	claimed := map[string]bool{}

	playerIDs := make([]string, 0, len(m.d.Transfers))
	for id := range m.d.Transfers {
		playerIDs = append(playerIDs, id)
	}
	sort.Strings(playerIDs)

	for _, sourcePlayer := range playerIDs {
		player, ok := m.players[sourcePlayer]
		if !ok {
			continue
		}
		seen := map[string]bool{}
		for _, mv := range Classify(m.d.Transfers[sourcePlayer]) {
			from, to := m.teamFor(mv.FromID), m.teamFor(mv.ToID)
			if from == nil && to == nil {
				continue
			}
			key := strings.Join([]string{sourcePlayer, mv.Date, mv.FromID, mv.ToID}, "|")
			if seen[key] {
				continue
			}
			seen[key] = true
			season := m.seasonOn(mv.Date)

			local, ok := m.ref("transfer", key)
			if ok {
				_, ok = byID[local]
			}
			if !ok {
				// The same move entered by hand: same clubs, within a week.
				for _, t := range byPlayer[player] {
					if claimed[t.ID] || refByLocal[t.ID] || !sameTeam(t.From, from) || !sameTeam(t.To, to) {
						continue
					}
					if daysApart(t.Date, mv.Date) <= 7 {
						local, ok = t.ID, true
						m.report.TransfersAdopted++
						break
					}
				}
			} else {
				m.report.TransfersUpdated++
			}

			if ok {
				claimed[local] = true
				updates.add(local, player, from, to, season, mv.Date, mv.Type, mv.FeeMinor)
			} else {
				local = uuid.NewString()
				m.report.TransfersCreated++
				inserts.add(local, player, from, to, season, mv.Date, mv.Type, mv.FeeMinor)
			}
			m.remember("transfer", key, local)
		}
	}

	if err := inChunks(len(inserts.ids), func(lo, hi int) error {
		_, err := m.tx.ExecContext(m.ctx, `
			INSERT INTO transfers (id, player_id, from_team_id, to_team_id, season_id, transfer_date, transfer_type, fee_minor, currency)
			SELECT i::uuid, p::uuid, f::uuid, t::uuid, s::uuid, d::date, k, fee, 'EUR'
			  FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $8::bigint[])
			       AS u(i, p, f, t, s, d, k, fee)`,
			inserts.args(lo, hi)...)
		return err
	}); err != nil {
		return err
	}
	if err := inChunks(len(updates.ids), func(lo, hi int) error {
		_, err := m.tx.ExecContext(m.ctx, `
			UPDATE transfers x
			   SET from_team_id = u.f::uuid, to_team_id = u.t::uuid, season_id = COALESCE(u.s::uuid, x.season_id),
			       transfer_date = u.d::date, transfer_type = u.k, fee_minor = u.fee
			  FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $8::bigint[])
			       AS u(i, p, f, t, s, d, k, fee)
			 WHERE x.id = u.i::uuid`,
			updates.args(lo, hi)...)
		return err
	}); err != nil {
		return err
	}

	// The placeholder arrival this app writes when a player is created by
	// hand -- from nowhere, on the day they were entered or their contract
	// began, no fee -- is not a real move, and the real history replaces it.
	created := map[string]localPlayer{}
	var all []localPlayer
	if err := m.tx.SelectContext(m.ctx, &all, `
		SELECT id::text AS id, name, date_of_birth::text AS date_of_birth, team_id::text AS team_id,
		       contract_start::text AS contract_start, created_at::date::text AS created
		  FROM players WHERE id = ANY($1::uuid[])`, pq.Array(locals)); err != nil {
		return err
	}
	for _, p := range all {
		created[p.ID] = p
	}
	var openings []string
	for _, t := range existing {
		p := created[t.PlayerID]
		if !m.matched[t.PlayerID] || claimed[t.ID] || refByLocal[t.ID] {
			continue
		}
		placeholderDate := t.Date == p.Created || (p.ContractStart.Valid && t.Date == p.ContractStart.String)
		if !t.From.Valid && t.To.Valid && t.Type == string(domain.TransferPermanent) && !t.Fee.Valid && placeholderDate {
			openings = append(openings, t.ID)
		}
	}
	if len(openings) > 0 {
		if _, err := m.tx.ExecContext(m.ctx, `DELETE FROM transfers WHERE id = ANY($1::uuid[])`, pq.Array(openings)); err != nil {
			return err
		}
	}
	m.report.OpeningsRemoved = len(openings)
	return nil
}

func (c *transferColumns) args(lo, hi int) []any {
	return []any{
		pq.Array(c.ids[lo:hi]), pq.Array(c.players[lo:hi]), pq.Array(c.from[lo:hi]), pq.Array(c.to[lo:hi]),
		pq.Array(c.seasons[lo:hi]), pq.Array(c.dates[lo:hi]), pq.Array(c.types[lo:hi]), pq.Array(c.fees[lo:hi]),
	}
}

// teamFor is the local club a source club id became, or nil when the club is
// outside the import -- which the schema reads as "outside the dataset".
func (m *merge) teamFor(clubID string) *string {
	if id, ok := m.clubs[clubID]; ok {
		return &id
	}
	return nil
}

func sameTeam(a sql.NullString, b *string) bool {
	if !a.Valid || b == nil {
		return !a.Valid && b == nil
	}
	return a.String == *b
}

func daysApart(a, b string) int {
	ta, errA := time.Parse("2006-01-02", a)
	tb, errB := time.Parse("2006-01-02", b)
	if errA != nil || errB != nil {
		return 1 << 30
	}
	d := ta.Sub(tb)
	if d < 0 {
		d = -d
	}
	return int(d.Hours() / 24)
}

// ----------------------------------------------------------- season stats

// stats replaces the season lines of every imported player. Replace rather
// than merge because a line is an aggregate: when the source corrects one
// match, the right answer is the new sum, not a reconciliation with the old.
func (m *merge) importStats() error {
	locals := make([]string, 0, len(m.players))
	for _, id := range m.players {
		locals = append(locals, id)
	}
	if err := inChunks(len(locals), func(lo, hi int) error {
		_, err := m.tx.ExecContext(m.ctx, `DELETE FROM player_season_stats WHERE player_id = ANY($1::uuid[])`, pq.Array(locals[lo:hi]))
		return err
	}); err != nil {
		return err
	}

	var players, seasons, leagues []string
	var teams []sql.NullString
	var apps, mins, goals, assists, yellow, red []int64
	for _, l := range m.d.Stats {
		player, okP := m.players[l.PlayerID]
		league, okL := m.comps[l.Competition]
		season := m.seasonID(l.Season)
		if !okP || !okL || season == nil {
			continue
		}
		players, seasons, leagues = append(players, player), append(seasons, *season), append(leagues, league)
		teams = append(teams, nullString(m.teamFor(l.ClubID)))
		apps, mins, goals = append(apps, int64(l.Appearances)), append(mins, int64(l.Minutes)), append(goals, int64(l.Goals))
		assists, yellow, red = append(assists, int64(l.Assists)), append(yellow, int64(l.Yellow)), append(red, int64(l.Red))
	}
	m.report.StatLines = len(players)
	return inChunks(len(players), func(lo, hi int) error {
		_, err := m.tx.ExecContext(m.ctx, `
			INSERT INTO player_season_stats (player_id, season_id, league_id, team_id,
			       appearances, minutes, goals, assists, yellow_cards, red_cards)
			SELECT p::uuid, s::uuid, l::uuid, t::uuid, a, mi, g, ast, y, r
			  FROM unnest($1::text[], $2::text[], $3::text[], $4::text[],
			              $5::int[], $6::int[], $7::int[], $8::int[], $9::int[], $10::int[])
			       AS u(p, s, l, t, a, mi, g, ast, y, r)`,
			pq.Array(players[lo:hi]), pq.Array(seasons[lo:hi]), pq.Array(leagues[lo:hi]), pq.Array(teams[lo:hi]),
			pq.Array(apps[lo:hi]), pq.Array(mins[lo:hi]), pq.Array(goals[lo:hi]),
			pq.Array(assists[lo:hi]), pq.Array(yellow[lo:hi]), pq.Array(red[lo:hi]))
		return err
	})
}

// ------------------------------------------------------ competition entries

func (m *merge) importEntries() error {
	var teams, seasons, leagues []string
	for _, e := range m.d.Entries {
		team, okT := m.clubs[e.ClubID]
		league, okL := m.comps[e.Competition]
		season := m.seasonID(e.Season)
		if !okT || !okL || season == nil {
			continue
		}
		teams, seasons, leagues = append(teams, team), append(seasons, *season), append(leagues, league)
	}
	m.report.Entries = len(teams)
	return inChunks(len(teams), func(lo, hi int) error {
		_, err := m.tx.ExecContext(m.ctx, `
			INSERT INTO team_seasons (team_id, season_id, league_id)
			SELECT t::uuid, s::uuid, l::uuid FROM unnest($1::text[], $2::text[], $3::text[]) AS u(t, s, l)
			ON CONFLICT (team_id, season_id, league_id) DO NOTHING`,
			pq.Array(teams[lo:hi]), pq.Array(seasons[lo:hi]), pq.Array(leagues[lo:hi]))
		return err
	})
}

// inChunks calls fn over [0, n) in slices small enough that one statement's
// parameters stay a few megabytes.
func inChunks(n int, fn func(lo, hi int) error) error {
	const size = 5000
	for lo := 0; lo < n; lo += size {
		if err := fn(lo, min(lo+size, n)); err != nil {
			return err
		}
	}
	return nil
}
