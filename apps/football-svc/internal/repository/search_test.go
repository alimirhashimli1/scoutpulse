package repository

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestToTSQuery_BuildsPrefixTerms pins the type-ahead behaviour.
//
// Full-text search matches whole lexemes, so without the ":*" suffix typing
// "mess" would find nothing at all until the final "i" — which is not what a
// search box is expected to do.
func TestToTSQuery_BuildsPrefixTerms(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"one word", "messi", "messi:*"},
		{"two words are both required", "lionel messi", "lionel:* & messi:*"},
		{"extra spacing is ignored", "  lionel   messi  ", "lionel:* & messi:*"},
		{"digits are kept", "atletico 1903", "atletico:* & 1903:*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ToTSQuery(tt.input))
		})
	}
}

// TestToTSQuery_StripsQueryOperators is the security-relevant case.
//
// to_tsquery takes an expression, not a literal, and the input cannot be
// passed as a parameter. A stray "&" or ")" would be a syntax error, and a
// crafted one could make the query pathologically expensive — so anything that
// is not a letter or digit is dropped rather than escaped.
func TestToTSQuery_StripsQueryOperators(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"ampersand", "messi & ronaldo"},
		{"pipe", "messi | ronaldo"},
		{"negation", "!messi"},
		{"parentheses", "(messi)"},
		{"existing prefix operator", "messi:*:*"},
		{"phrase operator", "messi <-> ronaldo"},
		{"quotes and backslashes", `"messi" \ 'x'`},
		{"semicolon and comment", "messi; -- drop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToTSQuery(tt.input)

			// Every term is a bare word followed by the prefix marker, joined
			// only by the operator this function supplies itself.
			for _, term := range strings.Split(got, " & ") {
				if term == "" {
					continue
				}
				assert.True(t, strings.HasSuffix(term, ":*"), "each term must be a prefix term, got %q", term)
				word := strings.TrimSuffix(term, ":*")
				assert.NotEmpty(t, word)
				for _, r := range word {
					assert.True(t, isLetterOrDigit(r),
						"query operators must be stripped, found %q in %q", r, got)
				}
			}
		})
	}
}

func isLetterOrDigit(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
		r > 127 // non-ASCII letters are kept; unicode.IsLetter allows them
}

// TestToTSQuery_EmptyWhenNothingSearchable: the repository uses this to skip
// the database entirely rather than issue a query that matches everything.
func TestToTSQuery_EmptyWhenNothingSearchable(t *testing.T) {
	for _, input := range []string{"", "   ", "&|!()", "--", "***"} {
		assert.Empty(t, ToTSQuery(input), "input %q should produce no query", input)
	}
}

// TestToTSQuery_BoundsTermLength: a pasted wall of text cannot become a very
// long prefix term, which would scan the index for no possible match.
func TestToTSQuery_BoundsTermLength(t *testing.T) {
	long := strings.Repeat("a", 500)

	got := ToTSQuery(long)

	assert.Equal(t, strings.Repeat("a", 64)+":*", got)
}

// TestToTSQuery_KeepsNonASCIILetters: this dataset is multilingual by nature,
// so stripping accents or non-Latin scripts would make those names unfindable.
func TestToTSQuery_KeepsNonASCIILetters(t *testing.T) {
	assert.Equal(t, "beşiktaş:*", ToTSQuery("beşiktaş"))
	assert.Equal(t, "mönchengladbach:*", ToTSQuery("mönchengladbach"))
	assert.Equal(t, "динамо:*", ToTSQuery("динамо"))
}
