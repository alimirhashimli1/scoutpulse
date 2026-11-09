package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/news"
	"github.com/scoutpulse/libs/platform/httpx"
)

// NewsHandler serves the football news feed and its moderation.
type NewsHandler struct {
	service *news.Service
}

func NewNewsHandler(service *news.Service) *NewsHandler {
	return &NewsHandler{service: service}
}

// publicCache lets browsers keep a page for a minute and a shared cache for
// two, serving a stale copy while it refetches. The feed changes at most
// every half hour, so this costs readers nothing and spares the database.
const publicCache = "public, max-age=60, s-maxage=120, stale-while-revalidate=300"

// List is GET /api/v1/news?category=&limit=&offset=.
func (h *NewsHandler) List(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.service.List(r.Context(), r.URL.Query().Get("category"), page.Limit, page.Offset)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", publicCache)
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Top is GET /api/v1/news/top?limit=.
func (h *NewsHandler) Top(w http.ResponseWriter, r *http.Request) {
	limit, err := httpx.QueryInt(r, "limit", 5)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.service.Top(r.Context(), limit)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", publicCache)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

// AdminList is GET /api/v1/admin/news.
func (h *NewsHandler) AdminList(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.service.AdminList(r.Context(), page.Limit, page.Offset)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Update is PATCH /api/v1/admin/news/{id} with {"hidden": bool, "pinned": bool}.
func (h *NewsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hidden *bool `json:"hidden"`
		Pinned *bool `json:"pinned"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	st, err := h.service.SetFlags(r.Context(), r.PathValue("id"), req.Hidden, req.Pinned)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, st)
}

// Run is POST /api/v1/admin/news/run.
func (h *NewsHandler) Run(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.RunNow(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, report)
}
