package repository

import (
	"context"
	"strings"
	"unicode"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

// SearchRepository answers "find me the thing called roughly this".
type SearchRepository interface {
	// Search returns ranked hits across players, clubs, coaches and
	// competitions. An empty kind searches all four.
	Search(ctx context.Context, query string, kind domain.SearchKind, page domain.Page) ([]domain.SearchResult, error)
}

type postgresSearchRepository struct {
	db *sqlx.DB
}

func NewPostgresSearchRepository(db *sqlx.DB) SearchRepository {
	return &postgresSearchRepository{db: db}
}

// ToTSQuery turns whatever the user typed into a safe prefix tsquery.
//
// It is not a parameter: to_tsquery takes an *expression* with its own
// operators (&, |, !, <->, :*, parentheses), so passing raw input would let a
// stray "&" or ")" produce a syntax error, and a crafted one could make the
// query pathologically slow. Everything that is not a letter or digit is
// therefore dropped rather than escaped, and the operators are supplied here.
//
// Each word gets ":*" so typing "mess" finds "Messi" — full-text search
// matches whole lexemes otherwise, which is not the behaviour anyone expects
// from a search box. Words are joined with "&" so extra terms narrow the
// result rather than widening it: "messi argentina" should mean both.
//
// Exported for testing; the rules above are worth pinning.
func ToTSQuery(input string) string {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		// A very long single "word" is almost always paste noise, and a
		// prefix search on it cannot match anything useful.
		if len(f) > 64 {
			f = f[:64]
		}
		terms = append(terms, f+":*")
	}

	return strings.Join(terms, " & ")
}

// Each branch produces the same five columns so the parts can be UNIONed.
//
// $1 is the tsquery, $2 the raw input used for the trigram tiebreak.
const (
	searchPlayers = `
		SELECT 'player' AS kind, p.id, p.name,
		       trim(both ' · ' FROM concat_ws(' · ', p.position, p.nationality)) AS subtitle,
		       p.team_id,
		       ts_rank(p.search_document, to_tsquery('simple', $1))
		         + similarity(p.name, $2) AS rank
		  FROM players p
		 WHERE p.search_document @@ to_tsquery('simple', $1)`

	searchTeams = `
		SELECT 'team' AS kind, t.id, t.name,
		       trim(both ' · ' FROM concat_ws(' · ', t.city, t.country)) AS subtitle,
		       NULL::uuid AS team_id,
		       ts_rank(t.search_document, to_tsquery('simple', $1))
		         + similarity(t.name, $2) AS rank
		  FROM teams t
		 WHERE t.search_document @@ to_tsquery('simple', $1)`

	searchCoaches = `
		SELECT 'coach' AS kind, c.id, c.name,
		       coalesce(c.nationality, '') AS subtitle,
		       c.team_id,
		       ts_rank(c.search_document, to_tsquery('simple', $1))
		         + similarity(c.name, $2) AS rank
		  FROM coaches c
		 WHERE c.search_document @@ to_tsquery('simple', $1)`

	searchLeagues = `
		SELECT 'league' AS kind, l.id, l.name,
		       coalesce(l.country, '') AS subtitle,
		       NULL::uuid AS team_id,
		       ts_rank(l.search_document, to_tsquery('simple', $1))
		         + similarity(l.name, $2) AS rank
		  FROM leagues l
		 WHERE l.search_document @@ to_tsquery('simple', $1)`
)

func (r *postgresSearchRepository) Search(ctx context.Context, query string, kind domain.SearchKind, page domain.Page) ([]domain.SearchResult, error) {
	tsquery := ToTSQuery(query)
	if tsquery == "" {
		// Nothing searchable was typed. Returning early keeps a query that
		// would match everything off the database.
		return nil, nil
	}

	var parts []string
	switch kind {
	case domain.SearchPlayer:
		parts = []string{searchPlayers}
	case domain.SearchTeam:
		parts = []string{searchTeams}
	case domain.SearchCoach:
		parts = []string{searchCoaches}
	case domain.SearchLeague:
		parts = []string{searchLeagues}
	default:
		parts = []string{searchPlayers, searchTeams, searchCoaches, searchLeagues}
	}

	// id breaks ties so paging is stable; without it two equally ranked rows
	// can swap between pages.
	sql := strings.Join(parts, "\nUNION ALL\n") +
		"\nORDER BY rank DESC, name, id\nLIMIT $3 OFFSET $4"

	var results []domain.SearchResult
	if err := r.db.SelectContext(ctx, &results, sql, tsquery, query, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("search", err)
	}
	return results, nil
}
