package importer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/scoutpulse/football-svc/internal/domain"
)

// Competition is how one Transfermarkt competition id appears here.
type Competition struct {
	Name    string
	Country string
	Type    domain.CompetitionType
	// League is the domestic league a cup belongs to, so a cup is imported
	// only alongside its league. Empty for leagues and continental cups.
	League string
}

// Catalog names the competitions an import can bring in. Display names are
// the ones a reader would recognise, not Transfermarkt's slugs ("laliga",
// "bundesliga" twice over).
var Catalog = map[string]Competition{
	"GB1": {Name: "Premier League", Country: "England", Type: domain.CompetitionLeague},
	"ES1": {Name: "LaLiga", Country: "Spain", Type: domain.CompetitionLeague},
	"IT1": {Name: "Serie A", Country: "Italy", Type: domain.CompetitionLeague},
	"L1":  {Name: "Bundesliga", Country: "Germany", Type: domain.CompetitionLeague},
	"FR1": {Name: "Ligue 1", Country: "France", Type: domain.CompetitionLeague},
	"PO1": {Name: "Liga Portugal", Country: "Portugal", Type: domain.CompetitionLeague},
	"NL1": {Name: "Eredivisie", Country: "Netherlands", Type: domain.CompetitionLeague},
	"BE1": {Name: "Jupiler Pro League", Country: "Belgium", Type: domain.CompetitionLeague},
	"TR1": {Name: "Süper Lig", Country: "Türkiye", Type: domain.CompetitionLeague},
	"TS1": {Name: "Chance Liga", Country: "Czech Republic", Type: domain.CompetitionLeague},
	"SC1": {Name: "Scottish Premiership", Country: "Scotland", Type: domain.CompetitionLeague},
	"A1":  {Name: "Austrian Bundesliga", Country: "Austria", Type: domain.CompetitionLeague},
	"GR1": {Name: "Super League Greece", Country: "Greece", Type: domain.CompetitionLeague},
	"DK1": {Name: "Danish Superliga", Country: "Denmark", Type: domain.CompetitionLeague},
	"C1":  {Name: "Swiss Super League", Country: "Switzerland", Type: domain.CompetitionLeague},

	"FAC": {Name: "FA Cup", Country: "England", Type: domain.CompetitionDomesticCup, League: "GB1"},
	"CDR": {Name: "Copa del Rey", Country: "Spain", Type: domain.CompetitionDomesticCup, League: "ES1"},
	"CIT": {Name: "Coppa Italia", Country: "Italy", Type: domain.CompetitionDomesticCup, League: "IT1"},
	"DFB": {Name: "DFB-Pokal", Country: "Germany", Type: domain.CompetitionDomesticCup, League: "L1"},
	"NLP": {Name: "KNVB Beker", Country: "Netherlands", Type: domain.CompetitionDomesticCup, League: "NL1"},
	"SFA": {Name: "Scottish Cup", Country: "Scotland", Type: domain.CompetitionDomesticCup, League: "SC1"},
	"GRP": {Name: "Greek Cup", Country: "Greece", Type: domain.CompetitionDomesticCup, League: "GR1"},
	"DKP": {Name: "Danish Cup", Country: "Denmark", Type: domain.CompetitionDomesticCup, League: "DK1"},

	"GBCS": {Name: "Community Shield", Country: "England", Type: domain.CompetitionSuperCup, League: "GB1"},
	"SUC":  {Name: "Supercopa de España", Country: "Spain", Type: domain.CompetitionSuperCup, League: "ES1"},
	"SCI":  {Name: "Supercoppa Italiana", Country: "Italy", Type: domain.CompetitionSuperCup, League: "IT1"},
	"DFL":  {Name: "Franz Beckenbauer Supercup", Country: "Germany", Type: domain.CompetitionSuperCup, League: "L1"},
	"FRCH": {Name: "Trophée des Champions", Country: "France", Type: domain.CompetitionSuperCup, League: "FR1"},
	"POSU": {Name: "Supertaça Cândido de Oliveira", Country: "Portugal", Type: domain.CompetitionSuperCup, League: "PO1"},
	"NLSC": {Name: "Johan Cruijff Schaal", Country: "Netherlands", Type: domain.CompetitionSuperCup, League: "NL1"},
	"BESC": {Name: "Belgian Super Cup", Country: "Belgium", Type: domain.CompetitionSuperCup, League: "BE1"},

	"CL":   {Name: "UEFA Champions League", Country: "Europe", Type: domain.CompetitionInternational},
	"EL":   {Name: "UEFA Europa League", Country: "Europe", Type: domain.CompetitionInternational},
	"UCOL": {Name: "UEFA Conference League", Country: "Europe", Type: domain.CompetitionInternational},
	"USC":  {Name: "UEFA Super Cup", Country: "Europe", Type: domain.CompetitionInternational},
}

// DefaultLeagues are the ten strongest domestic leagues by UEFA country
// coefficient going into 2026/27.
var DefaultLeagues = []string{"GB1", "IT1", "ES1", "L1", "FR1", "NL1", "PO1", "BE1", "TS1", "TR1"}

// competitionsFor is every catalog entry an import of these leagues brings
// in: the leagues, their domestic cups, and the continental competitions.
func competitionsFor(leagues []string) map[string]Competition {
	chosen := map[string]bool{}
	for _, l := range leagues {
		chosen[l] = true
	}
	out := map[string]Competition{}
	for id, c := range Catalog {
		switch {
		case c.Type == domain.CompetitionLeague && chosen[id],
			c.League != "" && chosen[c.League],
			c.Type == domain.CompetitionInternational:
			out[id] = c
		}
	}
	return out
}

// seasonLabel turns Transfermarkt's starting year into the label used here.
func seasonLabel(start int) string {
	return fmt.Sprintf("%d/%02d", start, (start+1)%100)
}

// --------------------------------------------------------------- names

var foldLetters = strings.NewReplacer(
	"ø", "o", "æ", "ae", "ß", "ss", "ı", "i", "ł", "l", "đ", "d", "œ", "oe", "þ", "th", "ð", "d",
)

// fold reduces a name to the form two spellings of it share: lower case, no
// accents, no punctuation, single spaces. "Atlético de Madrid" and "Atletico
// de Madrid" fold alike; so do "Ødegaard" and "Odegaard".
func fold(s string) string {
	s = foldLetters.Replace(strings.ToLower(s))
	var b strings.Builder
	space := false
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}

// clubAffixes are the legal-form tokens clubs are written with or without:
// "Arsenal" and "Arsenal FC", "Inter" and "FC Internazionale".
var clubAffixes = map[string]bool{
	"fc": true, "afc": true, "cf": true, "sc": true, "ac": true, "as": true, "ss": true, "ssc": true,
	"us": true, "acf": true, "sv": true, "vfb": true, "vfl": true, "tsg": true, "rcd": true, "rc": true,
	"ud": true, "cd": true, "sd": true, "ca": true, "fk": true, "sk": true, "bk": true, "kv": true,
	"krc": true, "rsc": true, "osc": true, "ogc": true, "losc": true, "sl": true, "club": true,
	"football": true, "calcio": true, "1": true, "de": true,
}

// clubKey folds a club name and drops its legal-form tokens.
func clubKey(s string) string {
	words := strings.Fields(fold(s))
	kept := words[:0]
	for _, w := range words {
		if !clubAffixes[w] {
			kept = append(kept, w)
		}
	}
	if len(kept) == 0 {
		return fold(s)
	}
	return strings.Join(kept, " ")
}

// ----------------------------------------------------------- transfers

// RawTransfer is one row of the source's transfer history.
type RawTransfer struct {
	PlayerID string
	Date     string // YYYY-MM-DD
	FromID   string
	ToID     string
	FromName string
	ToName   string
	// Fee in euros. Nil when the source has none, which is how it writes an
	// undisclosed fee and also a move between a club's own sides.
	Fee *int64
}

// Move is a transfer after classification.
type Move struct {
	RawTransfer
	Type domain.TransferType
	// Fee as stored: nil for undisclosed, 0 for free, otherwise the fee.
	FeeMinor *int64
}

var youthSide = regexp.MustCompile(`(?i)(\bU-?\d{2}\b|\byth\.?$|\byouth\b|\bjugend\b|\bjgd\b|\bprimavera\b|\bjuvenil\b|\bacademy\b|\bII$|\bB$|\breserves?\b)`)

// noClub are the source's placeholders for "not at a club".
func noClub(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "without club", "career break", "unknown", "retired", "ban":
		return true
	}
	return false
}

// loanWindow bounds how long after a free move its reversal still counts as
// the end of a loan rather than a second, separate transfer. Two-season
// loans are common; three are not.
const loanWindow = 2*365*24*time.Hour + 60*24*time.Hour

// Classify gives one player's moves a transfer type.
//
// The source does not mark loans. What it records is a move A→B followed by
// a later B→A, usually on 30 June, the outbound one free or for a loan fee.
// So a free outbound move that is reversed within loanWindow is a loan, and
// the reversal is its return. That reads Félix's 2023 Atlético→Chelsea→Atlético
// correctly and still leaves a permanent sale that is later bought back as two
// permanent moves, because a buy-back carries a fee.
func Classify(moves []RawTransfer) []Move {
	sorted := append([]RawTransfer(nil), moves...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })

	out := make([]Move, len(sorted))
	returnOf := map[int]bool{}
	for i, t := range sorted {
		out[i] = Move{RawTransfer: t}
		if returnOf[i] {
			continue
		}
		if t.Fee != nil && *t.Fee == 0 && t.FromID != "" && t.ToID != "" && !noClub(t.ToName) && !noClub(t.FromName) {
			start, _ := time.Parse("2006-01-02", t.Date)
			for j := i + 1; j < len(sorted); j++ {
				back := sorted[j]
				at, _ := time.Parse("2006-01-02", back.Date)
				if at.Sub(start) > loanWindow {
					break
				}
				if back.FromID == t.ToID && back.ToID == t.FromID && (back.Fee == nil || *back.Fee == 0) {
					returnOf[j] = true
					out[i].Type = domain.TransferLoan
					break
				}
				// The player moved on from the loan club to somewhere else:
				// whatever this was, it did not end in a return.
				if back.FromID == t.ToID {
					break
				}
			}
		}
	}

	for i := range out {
		m := &out[i]
		switch {
		case returnOf[i]:
			m.Type = domain.TransferLoanReturn
			zero := int64(0)
			m.FeeMinor = &zero
			continue
		case m.Type == domain.TransferLoan:
		case strings.EqualFold(strings.TrimSpace(m.ToName), "retired"):
			m.Type = domain.TransferRetired
		case noClub(m.ToName):
			m.Type = domain.TransferReleased
		case youthSide.MatchString(strings.TrimSpace(m.FromName)) && (m.Fee == nil || *m.Fee == 0):
			m.Type = domain.TransferYouthPromotion
		case m.Fee != nil && *m.Fee == 0:
			m.Type = domain.TransferFree
		default:
			m.Type = domain.TransferPermanent
		}
		if m.Fee != nil {
			minor := *m.Fee * 100
			m.FeeMinor = &minor
		}
	}
	return out
}
