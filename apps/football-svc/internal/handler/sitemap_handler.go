package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
)

// SitemapPageSize is how many URLs one sitemap file holds. The protocol's
// ceiling is 50,000; staying under it leaves room for the fixed pages and
// keeps each response a reasonable size.
const SitemapPageSize = 40000

// sitemapMaxAge lets the edge and the renderer reuse an answer for an hour.
// Crawlers fetch sitemaps rarely, and a new player appearing an hour late in
// one is harmless.
const sitemapMaxAge = "public, max-age=3600"

type sitemapStore interface {
	Counts(ctx context.Context) (map[string]int, error)
	List(ctx context.Context, kind string, limit, offset int) ([]repository.SitemapEntry, error)
}

// SitemapHandler feeds the frontend's /sitemap.xml: every public record's URL
// key, without the row data a list endpoint carries, so ~40k players is one
// query rather than four hundred paged ones.
type SitemapHandler struct {
	store sitemapStore
}

func NewSitemapHandler(store sitemapStore) *SitemapHandler {
	return &SitemapHandler{store: store}
}

// Index returns the number of public records of each kind, and the page size,
// so the caller can lay out a sitemap index.
//
//	GET /api/v1/sitemap
func (h *SitemapHandler) Index(w http.ResponseWriter, r *http.Request) {
	counts, err := h.store.Counts(r.Context())
	if err != nil {
		httpx.WriteError(w, r, apperr.Internal(err))
		return
	}
	w.Header().Set("Cache-Control", sitemapMaxAge)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"page_size": SitemapPageSize,
		"counts":    counts,
	})
}

// List returns one page of one kind's URL keys.
//
//	GET /api/v1/sitemap/player?page=0
func (h *SitemapHandler) List(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !repository.IsSitemapKind(kind) {
		httpx.WriteError(w, r, apperr.NotFound("no such sitemap"))
		return
	}
	page := 0
	if v := r.URL.Query().Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 1000 {
			httpx.WriteError(w, r, apperr.Invalid("page must be a non-negative integer"))
			return
		}
		page = n
	}
	entries, err := h.store.List(r.Context(), kind, SitemapPageSize, page*SitemapPageSize)
	if err != nil {
		httpx.WriteError(w, r, apperr.Internal(err))
		return
	}
	w.Header().Set("Cache-Control", sitemapMaxAge)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": entries})
}
