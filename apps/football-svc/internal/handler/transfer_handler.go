package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/football-svc/internal/service"
	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/httpx"
)

type TransferHandler struct {
	service service.TransferService
}

func NewTransferHandler(s service.TransferService) *TransferHandler {
	return &TransferHandler{service: s}
}

func (h *TransferHandler) ListTransfers(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	filter := repository.TransferFilter{
		PlayerID: httpx.QueryString(r, "player_id"),
		TeamID:   httpx.QueryString(r, "team_id"),
		SeasonID: httpx.QueryString(r, "season_id"),
	}

	if t := httpx.QueryString(r, "type"); t != nil {
		transferType := domain.TransferType(*t)
		if !domain.ValidTransferType(transferType) {
			httpx.WriteError(w, r, apperr.Invalid("unknown transfer type: "+*t))
			return
		}
		filter.Type = &transferType
	}

	if raw := r.URL.Query().Get("min_fee_minor"); raw != "" {
		minFee, err := httpx.QueryInt(r, "min_fee_minor", 0)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		fee := domain.Minor(minFee)
		filter.MinFee = &fee
	}

	transfers, err := h.service.ListTransfers(r.Context(), filter, page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(transfers, page))
}

// ListByPlayer serves one player's career history, newest move first.
func (h *TransferHandler) ListByPlayer(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("id")
	if playerID == "" {
		httpx.WriteError(w, r, apperr.Invalid("player id is required"))
		return
	}

	page, err := pageFrom(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	transfers, err := h.service.ListTransfers(r.Context(), repository.TransferFilter{PlayerID: &playerID}, page)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.NewListResult(transfers, page))
}

func (h *TransferHandler) GetTransfer(w http.ResponseWriter, r *http.Request) {
	transfer, err := h.service.GetTransfer(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, transfer)
}

func (h *TransferHandler) RecordTransfer(w http.ResponseWriter, r *http.Request) {
	var transfer domain.Transfer
	if err := httpx.DecodeJSON(w, r, &transfer); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err := h.service.RecordTransfer(r.Context(), &transfer); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, transfer)
}

func (h *TransferHandler) DeleteTransfer(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteTransfer(r.Context(), r.PathValue("id")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
