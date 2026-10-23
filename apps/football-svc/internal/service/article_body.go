package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/scoutpulse/libs/platform/apperr"
	"golang.org/x/text/unicode/norm"
)

// An article body is the editor's document: a tree of nodes, each with a
// type, optional attributes and marks, and either children or text. The
// shape is ProseMirror's, which the editor speaks natively.
//
// The service does not render it, but it does read all of it. Every node type
// must be one the reader knows how to draw, and every address in it -- a link,
// an image, an embedded board or video -- must be one that is safe to put in
// front of a reader. The reader checks again as it renders; this is what keeps
// a stored article from ever holding something it would have to refuse.

type articleNode struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []articleNode  `json:"content,omitempty"`
	Marks   []articleMark  `json:"marks,omitempty"`
	Text    string         `json:"text,omitempty"`
}

type articleMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// The blocks and inline marks a body may contain.
var (
	articleBlockTypes = map[string]bool{
		"doc": true, "paragraph": true, "heading": true, "text": true, "hardBreak": true,
		"bulletList": true, "orderedList": true, "listItem": true,
		"blockquote": true, "horizontalRule": true,
		// The three embeds this site adds to the editor.
		"figure": true, "tacticBoard": true, "video": true,
	}
	articleMarkTypes = map[string]bool{
		"bold": true, "italic": true, "strike": true, "underline": true, "link": true,
	}
)

// Bounds on a body. Generous for writing -- a long-read is a few thousand
// words -- and far below what would make the table a file store.
const (
	maxArticleDepth  = 12
	maxArticleNodes  = 20000
	maxArticleText   = 200_000 // runes of prose
	maxURLLength     = 2048
	articleExcerptAt = 280
	wordsPerMinute   = 230
)

var (
	youtubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	vimeoID   = regexp.MustCompile(`^[0-9]{6,12}$`)
)

// articleBodyFacts is what the service derives from a body on every write.
type articleBodyFacts struct {
	excerpt        string
	readingMinutes int
	words          int
}

// checkArticleBody validates a body and derives its excerpt and reading time.
func checkArticleBody(raw json.RawMessage) (articleBodyFacts, error) {
	var doc articleNode
	if err := json.Unmarshal(raw, &doc); err != nil {
		return articleBodyFacts{}, apperr.Invalid("the article body is not a document")
	}
	if doc.Type != "doc" {
		return articleBodyFacts{}, apperr.Invalid("the article body is not a document")
	}

	w := bodyWalker{}
	if err := w.walk(doc, 0); err != nil {
		return articleBodyFacts{}, err
	}

	minutes := (w.words + wordsPerMinute - 1) / wordsPerMinute
	if minutes < 1 {
		minutes = 1
	}
	return articleBodyFacts{
		excerpt:        excerptOf(w.paragraphs),
		readingMinutes: minutes,
		words:          w.words,
	}, nil
}

type bodyWalker struct {
	nodes      int
	runes      int
	words      int
	paragraphs []string
}

func (w *bodyWalker) walk(n articleNode, depth int) error {
	w.nodes++
	if w.nodes > maxArticleNodes {
		return apperr.Invalid("the article is too long to save")
	}
	if depth > maxArticleDepth {
		return apperr.Invalid("the article nests lists or quotes too deeply")
	}
	if !articleBlockTypes[n.Type] {
		return apperr.Invalid(fmt.Sprintf("the article contains a block this site cannot show (%q)", n.Type))
	}

	switch n.Type {
	case "text":
		w.runes += utf8.RuneCountInString(n.Text)
		if w.runes > maxArticleText {
			return apperr.Invalid("the article is too long to save")
		}
		w.words += len(strings.Fields(n.Text))
		for _, m := range n.Marks {
			if err := checkMark(m); err != nil {
				return err
			}
		}
	case "heading":
		if level, ok := number(n.Attrs["level"]); !ok || level < 2 || level > 4 {
			return apperr.Invalid("a heading must be level 2, 3 or 4; the title is level 1")
		}
	case "figure":
		if !SafeImageURL(str(n.Attrs["src"])) {
			return apperr.Invalid("an image must be an https:// address")
		}
		if href := str(n.Attrs["href"]); href != "" && !SafeLinkURL(href) {
			return apperr.Invalid("an image links to an address this site will not open")
		}
	case "tacticBoard":
		if !validSlug(str(n.Attrs["slug"])) {
			return apperr.Invalid("an embedded tactics board has no valid link")
		}
	case "video":
		id := str(n.Attrs["id"])
		switch str(n.Attrs["provider"]) {
		case "youtube":
			if !youtubeID.MatchString(id) {
				return apperr.Invalid("an embedded YouTube video has no valid id")
			}
		case "vimeo":
			if !vimeoID.MatchString(id) {
				return apperr.Invalid("an embedded Vimeo video has no valid id")
			}
		default:
			return apperr.Invalid("videos can be embedded from YouTube or Vimeo")
		}
	}

	for _, child := range n.Content {
		if err := w.walk(child, depth+1); err != nil {
			return err
		}
	}

	if n.Type == "paragraph" && len(w.paragraphs) < 4 {
		if text := strings.TrimSpace(plainText(n)); text != "" {
			w.paragraphs = append(w.paragraphs, text)
		}
	}
	return nil
}

func checkMark(m articleMark) error {
	if !articleMarkTypes[m.Type] {
		return apperr.Invalid(fmt.Sprintf("the article uses formatting this site cannot show (%q)", m.Type))
	}
	if m.Type == "link" && !SafeLinkURL(str(m.Attrs["href"])) {
		return apperr.Invalid("a link must be an http(s):// or mailto: address, or a page on this site")
	}
	return nil
}

// SafeLinkURL accepts what a reader may be sent to by clicking: a web
// address, an email address, or a path on this site. Everything else --
// javascript:, data:, a protocol-relative //host -- is refused, because a link
// in a published article carries the site's trust.
func SafeLinkURL(raw string) bool {
	if raw == "" || len(raw) > maxURLLength || strings.ContainsAny(raw, " \t\r\n") {
		return false
	}
	if strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//") && !strings.HasPrefix(raw, `/\`)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return u.Host != ""
	case "mailto":
		return u.Opaque != ""
	}
	return false
}

// SafeImageURL accepts only https images. Plain http would be blocked or
// flagged as mixed content on an https page, and any other scheme is either
// not an image or a way to smuggle one in.
func SafeImageURL(raw string) bool {
	if raw == "" || len(raw) > maxURLLength || strings.ContainsAny(raw, " \t\r\n") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && strings.EqualFold(u.Scheme, "https") && u.Host != ""
}

func plainText(n articleNode) string {
	if n.Type == "text" {
		return n.Text
	}
	if n.Type == "hardBreak" {
		return " "
	}
	var b strings.Builder
	for _, c := range n.Content {
		b.WriteString(plainText(c))
	}
	return b.String()
}

// excerptOf takes the opening paragraphs up to articleExcerptAt runes, cut at
// a word so a preview never ends mid-name.
func excerptOf(paragraphs []string) string {
	text := strings.Join(strings.Fields(strings.Join(paragraphs, " ")), " ")
	if utf8.RuneCountInString(text) <= articleExcerptAt {
		return text
	}
	runes := []rune(text)[:articleExcerptAt]
	cut := strings.LastIndexFunc(string(runes), unicode.IsSpace)
	if cut <= 0 {
		return string(runes) + "…"
	}
	return strings.TrimRightFunc(string(runes)[:cut], func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}) + "…"
}

// slugWords turns a title into the readable half of an address:
// "Atlético's low block, explained" -> "atleticos-low-block-explained".
// Accents are dropped rather than percent-encoded, so the link reads the same
// when it is pasted into a message.
func slugWords(title string) string {
	const maxRunes = 60
	var b strings.Builder
	n, dash := 0, false
	for _, r := range norm.NFKD.String(strings.ToLower(title)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// A combining accent left behind by the decomposition.
		case r == '\'' || r == '’':
			// "Atlético's" reads better as "atleticos" than "atletico-s".
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if n >= maxRunes {
				// Out of room mid-word: end on the last whole word instead.
				words := b.String()
				if i := strings.LastIndexByte(words, '-'); i > 20 && !dash {
					words = words[:i]
				}
				return words
			}
			if dash && b.Len() > 0 {
				b.WriteByte('-')
				n++
			}
			dash = false
			b.WriteRune(r)
			n++
		default:
			dash = true
		}
	}
	return b.String()
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func number(v any) (int, bool) {
	f, ok := v.(float64)
	if !ok || f != float64(int(f)) {
		return 0, false
	}
	return int(f), true
}
