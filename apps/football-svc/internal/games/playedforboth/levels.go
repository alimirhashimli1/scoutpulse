package playedforboth

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Level is a difficulty: which teams a pair may be drawn from.
//
// Defined once, here, and sent to the browser inside the dataset (see
// Served), so the same-screen game drawing in the browser and an online room
// drawing on the server cannot disagree about what "easy" means.
type Level struct {
	Code string `json:"code"`
	// Leagues whose clubs are in play. Empty means every league.
	Leagues []string `json:"leagues"`
	// FamousOnly builds pairs from well-known players only.
	FamousOnly bool `json:"famousOnly"`
	// Clubs, when set, narrows the clubs in play to these ids: the big
	// clubs, for the easy level. Leagues is then ignored for them.
	Clubs []string `json:"clubs"`
	// Nations that may stand on one side, by FIFA code. Empty means all.
	Nations []string `json:"nations"`
	// WiderChance is how often a pair matches a club in play with a smaller
	// club from WiderLeagues instead: the "rarely a lower team" of easy.
	WiderChance  float64  `json:"widerChance"`
	WiderLeagues []string `json:"widerLeagues"`
}

const (
	LevelEasy   = "easy"
	LevelMedium = "medium"
	LevelHard   = "hard"
)

// The leagues of medium: the big five and the next three by reputation.
var mainLeagues = []string{"eng", "esp", "ita", "ger", "fra", "por", "ned", "tur"}

// Levels in the order the game offers them.
var Levels = []Level{
	{
		Code:       LevelEasy,
		FamousOnly: true,
		Clubs: []string{
			"ars", "che", "liv", "mun", "mci", "tot", "new",
			"rma", "bar", "atm", "sev", "val",
			"juv", "int", "mil", "rom", "laz", "nap",
			"bay", "bvb", "lev",
			"psg", "mar", "lyo", "mon",
			"ben", "fcp", "spo", "ajx", "psv",
			"gal", "fen", "bjk", "cel",
		},
		Nations:      []string{"ESP", "FRA", "ENG", "ITA", "BRA", "GER", "ARG", "NED", "POR", "BEL", "CRO", "URU"},
		WiderChance:  0.15,
		WiderLeagues: mainLeagues,
	},
	{Code: LevelMedium, FamousOnly: true, Leagues: mainLeagues},
	{Code: LevelHard},
}

// LevelByCode finds a level, reporting whether the code is one.
func LevelByCode(code string) (Level, bool) {
	for _, l := range Levels {
		if l.Code == code {
			return l, true
		}
	}
	return Level{}, false
}

// Served is the dataset as the browser receives it: the embedded file with
// the levels added under "levels". Built once by the caller.
func Served() ([]byte, error) {
	levels, err := json.Marshal(Levels)
	if err != nil {
		return nil, err
	}
	body := bytes.TrimSpace(raw)
	if len(body) < 2 || body[len(body)-1] != '}' {
		return nil, fmt.Errorf("played-for-both dataset: not a JSON object")
	}
	out := make([]byte, 0, len(body)+len(levels)+12)
	out = append(out, body[:len(body)-1]...)
	out = append(out, `,"levels":`...)
	out = append(out, levels...)
	out = append(out, '}')
	if !json.Valid(out) {
		return nil, fmt.Errorf("played-for-both dataset: levels did not splice cleanly")
	}
	return out, nil
}
