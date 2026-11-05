package handler

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scoutpulse/football-svc/internal/games"
	"github.com/scoutpulse/football-svc/internal/games/playedforboth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gameMux(t *testing.T) *http.ServeMux {
	t.Helper()
	h, err := NewGameHandler(playedforboth.Raw())
	require.NoError(t, err)
	mux := http.NewServeMux()
	RegisterRoutes(mux, Handlers{Game: h})
	return mux
}

func get(mux http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

const datasetPath = "/api/v1/games/played-for-both/dataset"

func TestDataset_ReturnsTheFileWithCacheHeaders(t *testing.T) {
	rec := get(gameMux(t), datasetPath, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "public, max-age=86400", rec.Header().Get("Cache-Control"))
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Regexp(t, `^"[0-9a-f]{64}"$`, rec.Header().Get("ETag"))
	assert.Equal(t, playedforboth.Raw(), rec.Body.Bytes())

	var body struct {
		Leagues []json.RawMessage `json:"leagues"`
		Clubs   []json.RawMessage `json:"clubs"`
		Nations []json.RawMessage `json:"nations"`
		Players []json.RawMessage `json:"players"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Len(t, body.Leagues, 20)
	assert.Len(t, body.Clubs, 359)
	assert.Len(t, body.Nations, 60)
	assert.InDelta(t, 11200, len(body.Players), 300)
}

func TestDataset_NotModifiedOnMatchingETag(t *testing.T) {
	mux := gameMux(t)
	etag := get(mux, datasetPath, nil).Header().Get("ETag")

	rec := get(mux, datasetPath, map[string]string{"If-None-Match": etag})
	assert.Equal(t, http.StatusNotModified, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	assert.Equal(t, etag, rec.Header().Get("ETag"))

	// A stale tag gets the full body again.
	rec = get(mux, datasetPath, map[string]string{"If-None-Match": `"stale"`})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDataset_GzipWhenAccepted(t *testing.T) {
	mux := gameMux(t)
	rec := get(mux, datasetPath, map[string]string{"Accept-Encoding": "br, gzip"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	assert.Equal(t, "Accept-Encoding", rec.Header().Get("Vary"))
	assert.Less(t, rec.Body.Len(), len(playedforboth.Raw())/2)

	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	require.NoError(t, err)
	plain, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Equal(t, playedforboth.Raw(), plain)

	// The compressed tag revalidates too.
	etag := rec.Header().Get("ETag")
	rec = get(mux, datasetPath, map[string]string{"Accept-Encoding": "gzip", "If-None-Match": etag})
	assert.Equal(t, http.StatusNotModified, rec.Code)
}

func TestAcceptsGzip(t *testing.T) {
	cases := map[string]bool{
		"":                  false,
		"gzip":              true,
		"gzip, deflate, br": true,
		"br;q=1, gzip;q=0":  false,
		"*":                 true,
		"*, gzip;q=0":       false,
		"identity":          false,
	}
	for header, want := range cases {
		assert.Equal(t, want, acceptsGzip(header), header)
	}
}

func TestListGames(t *testing.T) {
	rec := get(gameMux(t), "/api/v1/games", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var list []games.Game
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.NotEmpty(t, list)
	assert.Equal(t, "played-for-both", list[0].Slug)
	assert.Equal(t, 2, list[0].Players)
	assert.Equal(t, games.StatusLive, list[0].Status)
}
