package news

import (
	"errors"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// trackingParams are query parameters that identify a campaign rather than a
// page. Two links that differ only in these are the same article.
var trackingParams = map[string]bool{
	"fbclid": true, "gclid": true, "dclid": true, "msclkid": true, "mc_cid": true,
	"mc_eid": true, "ocid": true, "cmpid": true, "ito": true, "ref": true, "src": true,
}

func isTracking(key string) bool {
	k := strings.ToLower(key)
	return trackingParams[k] || strings.HasPrefix(k, "utm_") || strings.HasPrefix(k, "at_")
}

// CanonicalURL reduces a link to the form that identifies the article: https,
// lowercase host without "www.", no fragment, no tracking parameters, the
// rest of the query sorted, no trailing slash. Only http(s) links are
// accepted, so a feed cannot smuggle a javascript: link into the page.
func CanonicalURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("not an http(s) link")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", errors.New("link has no host")
	}
	host = strings.TrimPrefix(host, "www.")
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		host += ":" + port
	}

	q := u.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		if !isTracking(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var query []string
	for _, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		for _, v := range vals {
			query = append(query, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}

	path := strings.TrimRight(u.EscapedPath(), "/")
	out := "https://" + host + path
	if len(query) > 0 {
		out += "?" + strings.Join(query, "&")
	}
	return out, nil
}

var tagPattern = regexp.MustCompile(`<[^>]*>`)

// CleanText turns feed text into one line of plain text: markup removed,
// entities decoded, control characters dropped, whitespace collapsed.
func CleanText(s string) string {
	s = tagPattern.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// clip shortens s to at most n runes, on a word boundary where possible.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}

// Normalize cleans one raw item. It reports false for an item that cannot be
// used: no title, no valid link, or no time.
func Normalize(src Source, sourceKey, sourceName, title, description, link string, published time.Time) (Article, bool) {
	title = clip(CleanText(title), 300)
	if title == "" || published.IsZero() {
		return Article{}, false
	}
	canonical, err := CanonicalURL(link)
	if err != nil {
		return Article{}, false
	}
	if sourceKey == "" {
		sourceKey = src.Key
	}
	if sourceName == "" {
		sourceName = src.Name
	}
	return Article{
		SourceKey:   sourceKey,
		SourceName:  clip(CleanText(sourceName), 80),
		Attribution: src.AttributionText,
		Supporting:  src.Supporting,
		Title:       title,
		Description: clip(CleanText(description), 600),
		URL:         canonical,
		Canonical:   canonical,
		PublishedAt: published.UTC(),
	}, true
}

// Dedupe keeps one article per canonical URL -- the one from a leading
// (non-supporting) source if there is one, else the earliest.
func Dedupe(articles []Article) []Article {
	index := make(map[string]int, len(articles))
	var out []Article
	for _, a := range articles {
		i, seen := index[a.Canonical]
		if !seen {
			index[a.Canonical] = len(out)
			out = append(out, a)
			continue
		}
		cur := out[i]
		if (cur.Supporting && !a.Supporting) ||
			(cur.Supporting == a.Supporting && a.PublishedAt.Before(cur.PublishedAt)) {
			out[i] = a
		}
	}
	return out
}

// InWindow drops items too old to take in, and items dated more than an hour
// ahead, which is a broken clock rather than news from the future.
func InWindow(articles []Article, now time.Time) []Article {
	var out []Article
	for _, a := range articles {
		if a.PublishedAt.After(now.Add(-IngestWindow)) && a.PublishedAt.Before(now.Add(time.Hour)) {
			out = append(out, a)
		}
	}
	return out
}
