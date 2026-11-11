package playedforboth

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Answer checking for online rooms, where the first player to type a right
// name wins the round and nobody is there to judge.
//
// A guess counts when it is the player's full name, their first name, or
// their surname -- the last word, or the last few words for a surname like
// "van Persie" or "De Bruyne". Case, accents and punctuation do not matter,
// and one slip of the keyboard is forgiven in a name of five letters or
// more (two from ten), because the race is about knowing, not spelling.

// MaxGuessLength bounds what a guess may be; no name is longer.
const MaxGuessLength = 60

// Letters that do not decompose into a base letter and a mark.
var folds = map[rune]string{
	'ø': "o", 'æ': "ae", 'œ': "oe", 'ß': "ss", 'ı': "i", 'ł': "l", 'đ': "d", 'ð': "d", 'þ': "th",
}

// words lowercases s, strips accents and splits it into words. Hyphens and
// dots separate words; apostrophes and anything else that is not a letter
// or digit disappear ("N'Golo" is "ngolo").
func words(s string) []string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// An accent, now split off its letter.
		case r == '-' || r == '.' || unicode.IsSpace(r):
			b.WriteByte(' ')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			r = unicode.ToLower(r)
			if f, ok := folds[r]; ok {
				b.WriteString(f)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return strings.Fields(b.String())
}

// accepted is every way of naming a player that counts, without spaces.
func accepted(name string) []string {
	w := words(name)
	if len(w) == 0 {
		return nil
	}
	out := []string{strings.Join(w, "")}
	if len(w) > 1 {
		out = append(out, w[0])
		for i := 1; i < len(w); i++ {
			out = append(out, strings.Join(w[i:], ""))
		}
	}
	// A two-letter fragment ("da", "Li") would match far too much.
	kept := out[:0]
	for _, a := range out {
		if len([]rune(a)) >= 3 {
			kept = append(kept, a)
		}
	}
	return kept
}

// NameMatches reports whether a typed guess names the player.
func NameMatches(guess, name string) bool {
	g := strings.Join(words(guess), "")
	if len([]rune(g)) < 3 {
		return false
	}
	for _, a := range accepted(name) {
		if near(g, a) {
			return true
		}
	}
	return false
}

func near(a, b string) bool {
	if a == b {
		return true
	}
	n := len([]rune(b))
	allowed := 0
	switch {
	case n >= 10:
		allowed = 2
	case n >= 5:
		allowed = 1
	}
	return allowed > 0 && within(a, b, allowed)
}

// within reports whether the edit distance between a and b is at most k.
func within(a, b string, k int) bool {
	ra, rb := []rune(a), []rune(b)
	if d := len(ra) - len(rb); d > k || -d > k {
		return false
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		best := cur[0]
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			best = min(best, cur[j])
		}
		if best > k {
			return false
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)] <= k
}

// FindAnswer returns the first of the players whose name the guess matches,
// as an index into the dataset's players.
func (ix *Index) FindAnswer(guess string, players []int) (int, bool) {
	// An exact full-name match beats a fuzzy surname one, so "Ronaldo"
	// typed for a pair with both Ronaldos credits the right name either way
	// but a precise answer is shown as typed.
	g := strings.Join(words(guess), "")
	for _, i := range players {
		if g != "" && strings.Join(words(ix.Data.Players[i].Name), "") == g {
			return i, true
		}
	}
	for _, i := range players {
		if NameMatches(guess, ix.Data.Players[i].Name) {
			return i, true
		}
	}
	return 0, false
}
