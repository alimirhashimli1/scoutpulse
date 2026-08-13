package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/httpx"
)

// SearchHandler serves the quick search that a name-entry box needs.
type SearchHandler struct {
	service service.SearchService
}

func NewSearchHandler(s service.SearchService) *SearchHandler {
	return &SearchHandler{service: s}
}

// Search returns ranked hits across players, clubs, coaches and competitions.
//
//	GET /api/v1/search?q=messi
//	GET /api/v1/search?q=united&kind=team&limit=10
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var kind domain.SearchKind
	if k := httpx.QueryString(r, "kind"); k != nil {
		kind = domain.SearchKind(*k)
	}

	results, err := h.service.Search(r.Context(), r.URL.Query().Get("q"), kind, page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(results, page))
}
