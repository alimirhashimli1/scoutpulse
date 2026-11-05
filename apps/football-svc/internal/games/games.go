// Package games describes the browser games the platform offers.
//
// The list lives on the server so the games hub is not a hardcoded page: a
// new game is an entry here plus its own route in the frontend.
package games

// Status says whether a game can be played yet.
type Status string

const (
	StatusLive Status = "live"
	StatusSoon Status = "soon"
)

// Game is one card on the hub. Title and description are translation keys,
// not text: the frontend owns the strings in every language it ships.
type Game struct {
	Slug           string `json:"slug"`
	TitleKey       string `json:"titleKey"`
	DescriptionKey string `json:"descriptionKey"`
	Players        int    `json:"players"`
	Status         Status `json:"status"`
}

// Catalog returns every game, in the order the hub shows them.
func Catalog() []Game {
	return []Game{
		{
			Slug:           "played-for-both",
			TitleKey:       "games.playedForBoth.title",
			DescriptionKey: "games.playedForBoth.description",
			Players:        2,
			Status:         StatusLive,
		},
	}
}
