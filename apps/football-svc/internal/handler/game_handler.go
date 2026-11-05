package handler

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/scoutpulse/football-svc/internal/games"
	"github.com/scoutpulse/libs/platform/httpx"
)

// GameHandler serves the games hub list and each game's static data.
//
// Neither touches the database. The dataset is an embedded file, so every
// byte of the response — the body, its gzip copy and its ETag — is computed
// once at construction and each request only copies it out.
type GameHandler struct {
	dataset staticJSON
}

// NewGameHandler prepares the dataset response. It fails rather than serving
// something half-built, so main can refuse to start.
func NewGameHandler(playedForBoth []byte) (*GameHandler, error) {
	s, err := newStaticJSON(playedForBoth)
	if err != nil {
		return nil, err
	}
	return &GameHandler{dataset: s}, nil
}

// ListGames returns every game the hub can show. Public.
func (h *GameHandler) ListGames(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, games.Catalog())
}

// PlayedForBothDataset returns the whole trivia dataset, as stored. Public.
func (h *GameHandler) PlayedForBothDataset(w http.ResponseWriter, r *http.Request) {
	h.dataset.serve(w, r)
}

// datasetMaxAge is how long a browser may reuse the file without asking. A
// day: it changes only with a deploy, and a revalidation after that is a 304.
const datasetMaxAge = 86400

// staticJSON is a fixed JSON body with everything a cache needs.
//
// There is no compression middleware in this stack and the gateway does not
// encode either, so the gzip copy is made here, once. ~430 KB on the wire
// otherwise, for a file that compresses to about a third of that.
type staticJSON struct {
	body []byte
	gz   []byte
	etag string // strong, sha256 of body
}

func newStaticJSON(body []byte) (staticJSON, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return staticJSON{}, err
	}
	if _, err := zw.Write(body); err != nil {
		return staticJSON{}, err
	}
	if err := zw.Close(); err != nil {
		return staticJSON{}, err
	}
	sum := sha256.Sum256(body)
	return staticJSON{
		body: body,
		gz:   buf.Bytes(),
		etag: `"` + hex.EncodeToString(sum[:]) + `"`,
	}, nil
}

func (s staticJSON) serve(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	// The gzip variant gets its own tag: a strong ETag names exact bytes, and a
	// cache must not answer an identity request with the compressed ones.
	gzipped := acceptsGzip(r.Header.Get("Accept-Encoding"))
	etag := s.etag
	if gzipped {
		etag = strings.TrimSuffix(s.etag, `"`) + `-gz"`
	}
	h.Set("ETag", etag)
	h.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", datasetMaxAge))
	h.Set("Vary", "Accept-Encoding")

	if etagMatches(r.Header.Get("If-None-Match"), s.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	h.Set("Content-Type", "application/json")
	body := s.body
	if gzipped {
		h.Set("Content-Encoding", "gzip")
		body = s.gz
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// etagMatches reports whether an If-None-Match header names this resource,
// in either encoding. Weak comparison, as RFC 9110 requires for this header.
func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	base := strings.Trim(etag, `"`)
	for _, candidate := range strings.Split(header, ",") {
		c := strings.TrimSpace(candidate)
		if c == "*" {
			return true
		}
		c = strings.Trim(strings.TrimPrefix(c, "W/"), `"`)
		if c == base || c == base+"-gz" {
			return true
		}
	}
	return false
}

// acceptsGzip reads Accept-Encoding. gzip named explicitly decides it, q=0
// included; otherwise a wildcard with a non-zero q allows it.
func acceptsGzip(header string) bool {
	wildcard := false
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		coding := strings.ToLower(strings.TrimSpace(fields[0]))
		q := 1.0
		for _, f := range fields[1:] {
			if v, ok := strings.CutPrefix(strings.TrimSpace(f), "q="); ok {
				if parsed, err := strconv.ParseFloat(v, 64); err == nil {
					q = parsed
				}
			}
		}
		switch coding {
		case "gzip", "x-gzip":
			return q > 0
		case "*":
			wildcard = q > 0
		}
	}
	return wildcard
}
