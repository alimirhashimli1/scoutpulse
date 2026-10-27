package importer

import (
	"compress/gzip"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The source: the open Transfermarkt dataset at
// github.com/dcaribou/transfermarkt-datasets, published as gzipped CSV and
// refreshed by its maintainers. Only the files and columns read here matter.
const DatasetURL = "https://pub-e682421888d945d684bcae8890b0ec20.r2.dev/data/"

// DatasetFiles are the files an import reads.
var DatasetFiles = []string{"clubs", "players", "transfers", "player_valuations", "games", "appearances"}

// Club is a club as the source has it.
type Club struct {
	ID         string
	Name       string
	League     string // domestic competition id
	Stadium    string
	LastSeason int
}

// Player is a player as the source has it.
type Player struct {
	ID                 string
	Name               string
	FirstName          string
	LastName           string
	Citizenship        string
	DateOfBirth        string // YYYY-MM-DD or empty
	Position           string
	SubPosition        string
	Foot               string
	HeightCM           int
	ContractUntil      string // YYYY-MM-DD or empty
	Agent              string
	ClubID             string
	MarketValueEUR     int64
	InternationalCaps  *int
	InternationalGoals *int
	LastSeason         int
}

// Valuation is one market value point.
type Valuation struct {
	PlayerID string
	Date     string
	EUR      int64
}

// StatLine is a player's aggregate for one competition, season and club.
type StatLine struct {
	PlayerID    string
	Season      int
	Competition string
	ClubID      string
	Appearances int
	Minutes     int
	Goals       int
	Assists     int
	Yellow      int
	Red         int
}

// Entry records that a club played in a competition in a season.
type Entry struct {
	ClubID      string
	Season      int
	Competition string
}

// Dataset is the slice of the source an import needs.
type Dataset struct {
	Clubs      map[string]Club
	Players    []Player
	Transfers  map[string][]RawTransfer // by player
	Valuations []Valuation
	Stats      []StatLine
	Entries    []Entry
	// LatestSeason is the most recent season the source covers, as the year
	// it starts in.
	LatestSeason int
}

// Scope is what to load.
type Scope struct {
	Leagues []string
	// FirstSeason is the earliest season whose stats and club entries are
	// read, as its starting year.
	FirstSeason int
	// Today drops transfers dated after it. The source lists moves already
	// agreed for the future -- the end of a loan next summer -- and those have
	// not happened yet.
	Today string
}

// Load reads the dataset from dir, keeping only what scope needs.
func Load(dir string, scope Scope) (*Dataset, error) {
	leagues := map[string]bool{}
	for _, l := range scope.Leagues {
		leagues[l] = true
	}
	competitions := competitionsFor(scope.Leagues)

	d := &Dataset{Clubs: map[string]Club{}, Transfers: map[string][]RawTransfer{}}

	// Clubs: every club that played in one of the leagues since FirstSeason,
	// so a season spent at a club since relegated still has a club to name.
	err := each(dir, "clubs", func(r row) error {
		c := Club{
			ID: r.str("club_id"), Name: r.str("name"), League: r.str("domestic_competition_id"),
			Stadium: r.str("stadium_name"), LastSeason: r.int("last_season"),
		}
		if leagues[c.League] && c.LastSeason >= scope.FirstSeason {
			d.Clubs[c.ID] = c
			d.LatestSeason = max(d.LatestSeason, c.LastSeason)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(d.Clubs) == 0 {
		return nil, errors.New("no clubs found for those leagues")
	}

	// Players: the current squads, which is every player the source last saw
	// at a club still in one of the leagues.
	wanted := map[string]bool{}
	err = each(dir, "players", func(r row) error {
		p := Player{
			ID: r.str("player_id"), Name: r.str("name"), FirstName: r.str("first_name"),
			LastName: r.str("last_name"), Citizenship: r.str("country_of_citizenship"),
			DateOfBirth: r.date("date_of_birth"), Position: r.str("position"),
			SubPosition: r.str("sub_position"), Foot: r.str("foot"), HeightCM: r.int("height_in_cm"),
			ContractUntil: r.date("contract_expiration_date"), Agent: r.str("agent_name"),
			ClubID: r.str("current_club_id"), MarketValueEUR: r.int64("market_value_in_eur"),
			InternationalCaps: r.optInt("international_caps"), InternationalGoals: r.optInt("international_goals"),
			LastSeason: r.int("last_season"),
		}
		club, ok := d.Clubs[p.ClubID]
		if ok && club.LastSeason == d.LatestSeason && p.LastSeason == d.LatestSeason {
			d.Players = append(d.Players, p)
			wanted[p.ID] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = each(dir, "transfers", func(r row) error {
		id := r.str("player_id")
		if !wanted[id] {
			return nil
		}
		t := RawTransfer{
			PlayerID: id, Date: r.date("transfer_date"),
			FromID: r.str("from_club_id"), ToID: r.str("to_club_id"),
			FromName: r.str("from_club_name"), ToName: r.str("to_club_name"),
			Fee: r.optInt64("transfer_fee"),
		}
		if t.Date != "" && t.Date <= scope.Today {
			d.Transfers[id] = append(d.Transfers[id], t)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = each(dir, "player_valuations", func(r row) error {
		if id := r.str("player_id"); wanted[id] {
			d.Valuations = append(d.Valuations, Valuation{PlayerID: id, Date: r.date("date"), EUR: r.int64("market_value_in_eur")})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Games give each appearance its season, and say which clubs contested
	// which competitions.
	gameSeason := map[string]int{}
	entries := map[Entry]bool{}
	err = each(dir, "games", func(r row) error {
		comp, season := r.str("competition_id"), r.int("season")
		if _, ok := competitions[comp]; !ok || season < scope.FirstSeason {
			return nil
		}
		gameSeason[r.str("game_id")] = season
		for _, side := range []string{"home_club_id", "away_club_id"} {
			if club := r.str(side); d.Clubs[club].ID != "" {
				entries[Entry{ClubID: club, Season: season, Competition: comp}] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for e := range entries {
		d.Entries = append(d.Entries, e)
	}

	type statKey struct {
		player, comp, club string
		season             int
	}
	lines := map[statKey]*StatLine{}
	err = each(dir, "appearances", func(r row) error {
		player := r.str("player_id")
		if !wanted[player] {
			return nil
		}
		season, ok := gameSeason[r.str("game_id")]
		if !ok {
			return nil
		}
		k := statKey{player: player, comp: r.str("competition_id"), club: r.str("player_club_id"), season: season}
		line := lines[k]
		if line == nil {
			line = &StatLine{PlayerID: player, Season: season, Competition: k.comp, ClubID: k.club}
			lines[k] = line
		}
		line.Appearances++
		line.Minutes += r.int("minutes_played")
		line.Goals += r.int("goals")
		line.Assists += r.int("assists")
		line.Yellow += r.int("yellow_cards")
		line.Red += r.int("red_cards")
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, l := range lines {
		d.Stats = append(d.Stats, *l)
	}

	return d, nil
}

// --------------------------------------------------------------- reading

type row struct {
	cols   map[string]int
	values []string
}

func (r row) str(name string) string {
	i, ok := r.cols[name]
	if !ok || i >= len(r.values) {
		return ""
	}
	return strings.TrimSpace(r.values[i])
}

// date keeps the YYYY-MM-DD part of a date or timestamp.
func (r row) date(name string) string {
	s := r.str(name)
	if len(s) >= 10 {
		return s[:10]
	}
	return ""
}

func (r row) optInt64(name string) *int64 {
	s := r.str(name)
	if s == "" {
		return nil
	}
	// Money and counts arrive as "35000000.000".
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || f < 0 {
		return nil
	}
	v := int64(math.Round(f))
	return &v
}

func (r row) int64(name string) int64 {
	if v := r.optInt64(name); v != nil {
		return *v
	}
	return 0
}

func (r row) optInt(name string) *int {
	v := r.optInt64(name)
	if v == nil {
		return nil
	}
	i := int(*v)
	return &i
}

func (r row) int(name string) int { return int(r.int64(name)) }

// each streams one gzipped CSV file, row by row. Streaming rather than
// reading whole: appearances alone is over a million rows.
func each(dir, name string, fn func(row) error) error {
	path := filepath.Join(dir, name+".csv.gz")
	//nolint:gosec // G304: the directory is the operator's own -data flag
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	reader := csv.NewReader(gz)
	reader.ReuseRecord = true
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read %s header: %w", path, err)
	}
	cols := make(map[string]int, len(header))
	for i, h := range header {
		cols[strings.TrimPrefix(h, string(rune(0xFEFF)))] = i
	}

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if err := fn(row{cols: cols, values: record}); err != nil {
			return err
		}
	}
}
