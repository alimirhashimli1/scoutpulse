package domain

// SearchKind is the sort of thing a search result points at.
type SearchKind string

const (
	SearchPlayer SearchKind = "player"
	SearchTeam   SearchKind = "team"
	SearchCoach  SearchKind = "coach"
	SearchLeague SearchKind = "league"
)

// ValidSearchKind reports whether k is a kind that can be searched. The empty
// string is allowed and means "all of them".
func ValidSearchKind(k SearchKind) bool {
	switch k {
	case "", SearchPlayer, SearchTeam, SearchCoach, SearchLeague:
		return true
	default:
		return false
	}
}

// SearchResult is one hit, in the shape a quick-search dropdown needs.
//
// It deliberately carries only enough to render a row and navigate: the kind
// tells the client which endpoint to follow, and Subtitle disambiguates two
// people with the same name. Anything more belongs in the entity's own
// endpoint, which the client fetches once the user picks a result.
type SearchResult struct {
	Kind SearchKind `db:"kind" json:"kind"`
	ID   string     `db:"id" json:"id"`
	Name string     `db:"name" json:"name"`
	// Subtitle is a short human line — a player's position and nationality, a
	// club's city and country. Empty when nothing useful is recorded.
	Subtitle string `db:"subtitle" json:"subtitle,omitempty"`
	// TeamID is set for players and coaches, so a result can link straight to
	// the club without a second request.
	TeamID *string `db:"team_id" json:"team_id,omitempty"`
	// Rank is the relevance score results are ordered by. Exposed because a
	// client showing mixed kinds may want to group or threshold on it.
	Rank float64 `db:"rank" json:"rank"`
}
