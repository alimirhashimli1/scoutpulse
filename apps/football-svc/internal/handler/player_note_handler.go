package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
)

type PlayerNoteHandler struct {
	service service.PlayerNoteService
}

func NewPlayerNoteHandler(service service.PlayerNoteService) *PlayerNoteHandler {
	return &PlayerNoteHandler{service: service}
}

// noteRequest is the whole writable surface of a note.
//
// Only the body. The author comes from the token -- accepting a name here
// would let anyone sign a note as somebody else -- and the player comes from
// the path.
type noteRequest struct {
	Body string `json:"body"`
}

// ListNotes returns a player's notes, newest first. Public.
func (h *PlayerNoteHandler) ListNotes(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	notes, err := h.service.ListByPlayer(r.Context(), r.PathValue("id"), page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(notes, page))
}

// MyNote returns the caller's own note on a player, so the form can open with
// their existing text rather than blank.
//
// A 404 here is the normal case for someone who has not written one, and the
// frontend reads it as "show an empty form".
func (h *PlayerNoteHandler) MyNote(w http.ResponseWriter, r *http.Request) {
	note, err := h.service.Mine(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, note)
}

func (h *PlayerNoteHandler) WriteNote(w http.ResponseWriter, r *http.Request) {
	var req noteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	note, err := h.service.Write(r.Context(), r.PathValue("id"), req.Body)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, note)
}

func (h *PlayerNoteHandler) EditNote(w http.ResponseWriter, r *http.Request) {
	noteID := r.PathValue("noteID")
	if noteID == "" {
		httpx.WriteError(w, r, apperr.Invalid("note id is required"))
		return
	}

	var req noteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	note, err := h.service.Edit(r.Context(), noteID, req.Body)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, note)
}

func (h *PlayerNoteHandler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), r.PathValue("noteID")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
