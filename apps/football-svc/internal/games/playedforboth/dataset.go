// Package playedforboth holds the data behind the "Who Played For Both?"
// trivia game.
//
// The dataset is a static file, extracted once from the standalone reference
// page by tools/played-for-both/extract-dataset.mjs and embedded in the
// binary. All game logic runs in the browser; the service only serves the
// file. It is parsed at startup anyway, so a corrupt or truncated file stops
// the service from booting instead of shipping a game that breaks on load.
package playedforboth

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed data/dataset.json
var raw []byte

// Dataset is the parsed file. On the wire every record is a positional array
// to keep the download small; these types give each position a name.
type Dataset struct {
	Leagues []League `json:"leagues"`
	Clubs   []Club   `json:"clubs"`
	Nations []Nation `json:"nations"`
	Players []Player `json:"players"`
}

// League is [code, country, leagueName].
type League struct {
	Code, Country, Name string
}

// Club is [id, name, colour1, colour2, leagueIndex].
type Club struct {
	ID, Name, Colour1, Colour2 string
	League                     int
}

// Nation is [fifaCode, nameEN, nameTR, flagIconsCode].
type Nation struct {
	FIFA, NameEN, NameTR, Flag string
}

// Player is [name, nationIndex (-1 = none), [clubIndexes], famous 0|1].
type Player struct {
	Name   string
	Nation int
	Clubs  []int
	Famous bool
}

func (l *League) UnmarshalJSON(b []byte) error {
	return tuple(b, "league", &l.Code, &l.Country, &l.Name)
}

func (c *Club) UnmarshalJSON(b []byte) error {
	return tuple(b, "club", &c.ID, &c.Name, &c.Colour1, &c.Colour2, &c.League)
}

func (n *Nation) UnmarshalJSON(b []byte) error {
	return tuple(b, "nation", &n.FIFA, &n.NameEN, &n.NameTR, &n.Flag)
}

func (p *Player) UnmarshalJSON(b []byte) error {
	var famous int
	if err := tuple(b, "player", &p.Name, &p.Nation, &p.Clubs, &famous); err != nil {
		return err
	}
	if famous != 0 && famous != 1 {
		return fmt.Errorf("player %q: famous must be 0 or 1, got %d", p.Name, famous)
	}
	p.Famous = famous == 1
	return nil
}

// tuple decodes a JSON array of exactly len(dst) elements, positionally.
func tuple(b []byte, what string, dst ...any) error {
	var parts []json.RawMessage
	if err := json.Unmarshal(b, &parts); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if len(parts) != len(dst) {
		return fmt.Errorf("%s: want %d fields, got %d", what, len(dst), len(parts))
	}
	for i, p := range parts {
		if err := json.Unmarshal(p, dst[i]); err != nil {
			return fmt.Errorf("%s field %d: %w", what, i, err)
		}
	}
	return nil
}

// Raw returns the embedded file exactly as it is served. Callers must not
// modify it.
func Raw() []byte { return raw }

// Load parses and checks the embedded file. Every index must point inside the
// array it refers to; the browser trusts them blindly.
func Load() (*Dataset, error) {
	var d Dataset
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("played-for-both dataset: %w", err)
	}
	if len(d.Leagues) == 0 || len(d.Clubs) == 0 || len(d.Nations) == 0 || len(d.Players) == 0 {
		return nil, fmt.Errorf("played-for-both dataset: a section is empty")
	}
	for _, c := range d.Clubs {
		if c.League < 0 || c.League >= len(d.Leagues) {
			return nil, fmt.Errorf("played-for-both dataset: club %q has league %d", c.ID, c.League)
		}
	}
	for _, p := range d.Players {
		if p.Nation < -1 || p.Nation >= len(d.Nations) {
			return nil, fmt.Errorf("played-for-both dataset: player %q has nation %d", p.Name, p.Nation)
		}
		for _, c := range p.Clubs {
			if c < 0 || c >= len(d.Clubs) {
				return nil, fmt.Errorf("played-for-both dataset: player %q has club %d", p.Name, c)
			}
		}
	}
	return &d, nil
}
