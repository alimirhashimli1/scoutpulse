package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/httpx"
)

type ArticleHandler struct {
	service service.ArticleService
}

func NewArticleHandler(service service.ArticleService) *ArticleHandler {
	return &ArticleHandler{service: service}
}

// articleRequest is the writable surface of an article.
type articleRequest struct {
	Title        string          `json:"title"`
	Subtitle     string          `json:"subtitle"`
	CoverURL     string          `json:"cover_url"`
	CoverCaption string          `json:"cover_caption"`
	Body         json.RawMessage `json:"body"`
	Tags         []string        `json:"tags"`
}

func (req articleRequest) input() service.ArticleInput {
	return service.ArticleInput{
		Title: req.Title, Subtitle: req.Subtitle, CoverURL: req.CoverURL,
		CoverCaption: req.CoverCaption, Body: req.Body, Tags: req.Tags,
	}
}

// --- public ----------------------------------------------------------------

// ListArticles returns live articles, newest first. ?tag= narrows it to one
// topic.
func (h *ArticleHandler) ListArticles(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var tag *string
	if t := r.URL.Query().Get("tag"); t != "" {
		tag = &t
	}
	items, err := h.service.ListLive(r.Context(), tag, page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(items, page))
}

func (h *ArticleHandler) GetArticle(w http.ResponseWriter, r *http.Request) {
	a, err := h.service.GetLive(r.Context(), r.PathValue("slug"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

// --- the editorial desk ----------------------------------------------------

// DeskList returns articles in every state. ?status= narrows it; ?mine=true
// narrows an administrator's to their own.
func (h *ArticleHandler) DeskList(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	q := r.URL.Query()
	items, err := h.service.DeskList(r.Context(), service.ArticleDeskQuery{
		Status: q.Get("status"),
		Mine:   q.Get("mine") == "true",
	}, page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(items, page))
}

func (h *ArticleHandler) DeskGet(w http.ResponseWriter, r *http.Request) {
	a, err := h.service.DeskGet(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *ArticleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req articleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	a, err := h.service.Create(r.Context(), req.input())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, a)
}

func (h *ArticleHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req articleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	a, err := h.service.Update(r.Context(), r.PathValue("id"), req.input())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

// Submit takes an optional publish_at. Absent or past means as soon as
// possible: on approval for an editor, at once for an administrator.
func (h *ArticleHandler) Submit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublishAt *time.Time `json:"publish_at"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	a, err := h.service.Submit(r.Context(), r.PathValue("id"), req.PublishAt)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *ArticleHandler) RequestChanges(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Note string `json:"note"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	a, err := h.service.RequestChanges(r.Context(), r.PathValue("id"), req.Note)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *ArticleHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	a, err := h.service.Withdraw(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *ArticleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), r.PathValue("id")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
