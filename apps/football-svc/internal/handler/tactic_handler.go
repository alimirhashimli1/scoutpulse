package handler

import (
	"encoding/json"
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/httpx"
)

type TacticHandler struct {
	service service.TacticService
}

func NewTacticHandler(service service.TacticService) *TacticHandler {
	return &TacticHandler{service: service}
}

// tacticRequest is the whole writable surface of a board. The owner comes from
// the token and the slug from the server.
type tacticRequest struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Content     json.RawMessage `json:"content"`
}

func (req tacticRequest) input() service.TacticInput {
	return service.TacticInput{Title: req.Title, Description: req.Description, Content: req.Content}
}

// GetTactic returns one board with its document. Public: the link is the key.
func (h *TacticHandler) GetTactic(w http.ResponseWriter, r *http.Request) {
	board, err := h.service.Get(r.Context(), r.PathValue("slug"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, board)
}

// ListTactics returns board summaries grouped by author. ?author_id= narrows
// it to one member, which is how "my boards" is asked for.
func (h *TacticHandler) ListTactics(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var ownerID *string
	if id := r.URL.Query().Get("author_id"); id != "" {
		ownerID = &id
	}

	boards, err := h.service.List(r.Context(), ownerID, page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(boards, page))
}

func (h *TacticHandler) CreateTactic(w http.ResponseWriter, r *http.Request) {
	var req tacticRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	board, err := h.service.Create(r.Context(), req.input())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, board)
}

func (h *TacticHandler) UpdateTactic(w http.ResponseWriter, r *http.Request) {
	var req tacticRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	board, err := h.service.Update(r.Context(), r.PathValue("slug"), req.input())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, board)
}

func (h *TacticHandler) DeleteTactic(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), r.PathValue("slug")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
