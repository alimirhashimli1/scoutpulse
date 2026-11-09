package news

import (
	"sort"
	"strings"
	"unicode"
)

// stopwords carry no information about which story a headline is about.
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "of": true, "to": true,
	"in": true, "on": true, "for": true, "at": true, "by": true, "with": true, "from": true,
	"as": true, "is": true, "are": true, "was": true, "be": true, "has": true, "have": true,
	"after": true, "over": true, "into": true, "his": true, "her": true, "their": true,
	"it": true, "its": true, "that": true, "this": true, "will": true, "says": true,
	"say": true, "said": true, "new": true, "v": true, "vs": true, "up": true,
}

// generic words are football vocabulary every story shares. Two headlines
// alike only in these -- "X complete signing", "Y complete signing" -- are two
// stories, so a match also needs shared distinctive words: names, mostly.
var generic = map[string]bool{
	"complete": true, "sign": true, "signing": true, "deal": true, "transfer": true, "move": true,
	"join": true, "loan": true, "fee": true, "contract": true, "win": true, "won": true, "beat": true,
	"draw": true, "defeat": true, "victory": true, "match": true, "game": true, "goal": true,
	"score": true, "manager": true, "coach": true, "boss": true, "player": true, "club": true,
	"injury": true, "injur": true, "out": true, "season": true, "league": true, "cup": true,
	"final": true, "first": true, "last": true, "two": true, "three": true, "year": true,
	"month": true, "week": true, "football": true, "striker": true, "defender": true,
	"midfielder": true, "winger": true, "goalkeeper": true, "forward": true, "star": true,
	"report": true, "news": true, "confirm": true, "agree": true, "set": true, "against": true,
}

// titleTokens is the set of meaningful words in a headline.
func titleTokens(title string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if len(w) < 2 || stopwords[w] {
			continue
		}
		// A crude stem, so "signs" and "signing" meet "sign".
		for _, suf := range []string{"ing", "es", "s", "ed"} {
			if len(w) > len(suf)+3 && strings.HasSuffix(w, suf) {
				w = strings.TrimSuffix(w, suf)
				break
			}
		}
		set[w] = true
	}
	return set
}

// Similarity scores how alike two headlines are, 0 to 1: the larger of the
// Jaccard index and a damped overlap coefficient. The overlap term is what
// lets a short headline match a long one that contains it.
func Similarity(a, b string) float64 {
	ta, tb := titleTokens(a), titleTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	shared, distinct := 0, 0
	for w := range ta {
		if tb[w] {
			shared++
			if !generic[w] {
				distinct++
			}
		}
	}
	distinctA, distinctB := 0, 0
	for w := range ta {
		if !generic[w] {
			distinctA++
		}
	}
	for w := range tb {
		if !generic[w] {
			distinctB++
		}
	}
	if distinct < min(2, distinctA, distinctB) {
		return 0
	}
	union := len(ta) + len(tb) - shared
	jaccard := float64(shared) / float64(union)
	smaller := min(len(ta), len(tb))
	overlap := 0.0
	if smaller >= 3 {
		overlap = float64(shared) / float64(smaller) * 0.85
	}
	return max(jaccard, overlap)
}

// SameStoryThreshold is how alike two headlines must be to be one story.
const SameStoryThreshold = 0.5

// Group holds the articles of one story found in a run: either a stored story
// they joined (StoryID set) or a new one.
type Group struct {
	StoryID  string
	Existing *StoryRef
	Articles []Article
}

func matches(title string, titles []string) bool {
	for _, t := range titles {
		if Similarity(title, t) >= SameStoryThreshold {
			return true
		}
	}
	return false
}

// Cluster assigns each new article to a stored story it matches, or to a new
// group with other new articles it matches. Articles are taken oldest first,
// so the earliest report of a story is the one that starts it.
func Cluster(articles []Article, existing []StoryRef) []Group {
	sorted := append([]Article(nil), articles...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].PublishedAt.Before(sorted[j].PublishedAt) })

	var groups []Group
	byStory := map[string]int{}
	for _, a := range sorted {
		placed := false
		for i := range existing {
			if matches(a.Title, existing[i].Titles) {
				idx, ok := byStory[existing[i].ID]
				if !ok {
					idx = len(groups)
					byStory[existing[i].ID] = idx
					groups = append(groups, Group{StoryID: existing[i].ID, Existing: &existing[i]})
				}
				groups[idx].Articles = append(groups[idx].Articles, a)
				placed = true
				break
			}
		}
		if placed {
			continue
		}
		for i := range groups {
			if groups[i].StoryID != "" {
				continue
			}
			titles := make([]string, len(groups[i].Articles))
			for j, g := range groups[i].Articles {
				titles[j] = g.Title
			}
			if matches(a.Title, titles) {
				groups[i].Articles = append(groups[i].Articles, a)
				placed = true
				break
			}
		}
		if !placed {
			groups = append(groups, Group{Articles: []Article{a}})
		}
	}
	return groups
}

// Lead picks the article a new story is shown as: the earliest from a
// leading source, or the earliest overall if every one is supporting.
func Lead(articles []Article) Article {
	best := -1
	for i, a := range articles {
		if best < 0 {
			best = i
			continue
		}
		b := articles[best]
		if (b.Supporting && !a.Supporting) ||
			(b.Supporting == a.Supporting && a.PublishedAt.Before(b.PublishedAt)) {
			best = i
		}
	}
	return articles[best]
}

// Outlets counts distinct outlets across stored keys and new articles.
func Outlets(stored []string, articles []Article) int {
	set := map[string]bool{}
	for _, k := range stored {
		set[k] = true
	}
	for _, a := range articles {
		set[a.SourceKey] = true
	}
	return len(set)
}
