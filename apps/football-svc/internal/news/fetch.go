package news

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Fetcher reads one source.
type Fetcher interface {
	Fetch(ctx context.Context, src Source) ([]Article, error)
}

// maxFeedBytes bounds what one feed may send: a feed is tens of kilobytes,
// and an unbounded read is a way to exhaust memory.
const maxFeedBytes = 2 << 20

const userAgent = "ScoutPulseNewsBot/1.0 (+headlines and links only)"

func get(ctx context.Context, client *http.Client, rawURL string, header http.Header) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := client.Do(req)
	if err != nil {
		// The client's error embeds the URL; strip it so a query-string
		// secret can never reach a log line.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			return nil, uerr.Err
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxFeedBytes {
		return nil, errors.New("response too large")
	}
	if resp.StatusCode != http.StatusOK {
		return body, fmt.Errorf("status %d", resp.StatusCode)
	}
	return body, nil
}

// --- RSS -------------------------------------------------------------------

// RSSFetcher reads RSS 2.0 feeds. It reads only the feed: never the article
// pages it links to, and never its images.
type RSSFetcher struct {
	Client *http.Client
}

type rssDoc struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			GUID        string `xml:"guid"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

var rssTimeLayouts = []string{
	time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST", "2 Jan 2006 15:04:05 -0700", time.RFC3339,
}

func parseRSSTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range rssTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ParseRSS turns a feed document into articles.
func ParseRSS(src Source, body []byte) ([]Article, error) {
	var doc rssDoc
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	var out []Article
	for _, it := range doc.Channel.Items {
		link := it.Link
		if link == "" {
			link = it.GUID
		}
		if a, ok := Normalize(src, "", "", it.Title, it.Description, link, parseRSSTime(it.PubDate)); ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f RSSFetcher) Fetch(ctx context.Context, src Source) ([]Article, error) {
	body, err := get(ctx, f.Client, src.URL, nil)
	if err != nil {
		return nil, err
	}
	return ParseRSS(src, body)
}

// --- NewsData.io -----------------------------------------------------------

// NewsDataFetcher reads NewsData.io's latest-news endpoint. Each request
// costs one credit and returns up to ten articles; the free plan allows 200
// credits a day, and Credits holds this service well under that.
type NewsDataFetcher struct {
	Client  *http.Client
	APIKey  string
	Credits *Budget
}

type newsDataResponse struct {
	Status  string `json:"status"`
	Results []struct {
		Title       string `json:"title"`
		Link        string `json:"link"`
		Description string `json:"description"`
		PubDate     string `json:"pubDate"`
		SourceID    string `json:"source_id"`
		SourceName  string `json:"source_name"`
	} `json:"results"`
}

// ErrNoAPIKey is returned for NewsData with no key configured.
var ErrNoAPIKey = errors.New("NEWSDATA_API_KEY is not set")

// ParseNewsData turns a NewsData response into articles.
func ParseNewsData(src Source, body []byte) ([]Article, error) {
	var resp newsDataResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("status %q", resp.Status)
	}
	var out []Article
	for _, r := range resp.Results {
		t, err := time.Parse("2006-01-02 15:04:05", r.PubDate)
		if err != nil {
			continue
		}
		key := "newsdata"
		if id := strings.ToLower(strings.TrimSpace(r.SourceID)); id != "" {
			key = "newsdata:" + id
		}
		name := r.SourceName
		if name == "" {
			name = r.SourceID
		}
		if a, ok := Normalize(src, key, name, r.Title, r.Description, r.Link, t); ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f NewsDataFetcher) Fetch(ctx context.Context, src Source) ([]Article, error) {
	if f.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	if f.Credits != nil {
		if err := f.Credits.Reserve(ctx, 1); err != nil {
			return nil, err
		}
	}
	q := url.Values{}
	q.Set("q", "football OR soccer")
	q.Set("category", "sports")
	q.Set("language", "en")
	// The key goes in a header, not the query string, so it is never part of
	// a URL that might be logged.
	body, err := get(ctx, f.Client, src.URL+"?"+q.Encode(), http.Header{"X-Access-Key": {f.APIKey}})
	if err != nil {
		return nil, err
	}
	return ParseNewsData(src, body)
}

// --- dispatch ---------------------------------------------------------------

// Fetchers routes each source to the adapter for its type.
type Fetchers map[SourceType]Fetcher

func (fs Fetchers) Fetch(ctx context.Context, src Source) ([]Article, error) {
	f, ok := fs[src.Type]
	if !ok {
		return nil, fmt.Errorf("no adapter for source type %q", src.Type)
	}
	return f.Fetch(ctx, src)
}
