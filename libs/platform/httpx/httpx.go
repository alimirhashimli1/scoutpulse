// Package httpx holds the request/response helpers shared by service
// handlers: JSON encoding, request decoding, and the single place where a
// domain error becomes an HTTP status.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/scoutpulse/libs/platform/apperr"
	"github.com/scoutpulse/libs/platform/server"
)

// MaxRequestBodyBytes caps decoded request bodies so a single client cannot
// exhaust memory with an unbounded upload.
const MaxRequestBodyBytes = 1 << 20 // 1 MiB

// ErrorResponse is the body returned for every failed request.
type ErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
}

// StatusFor maps a domain error kind to an HTTP status code. This is the only
// such mapping in the codebase; handlers must not write their own.
func StatusFor(kind apperr.Kind) int {
	switch kind {
	case apperr.KindInvalid:
		return http.StatusBadRequest
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// WriteJSON serializes v as the response body.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v == nil || status == http.StatusNoContent {
		return
	}

	// The status line is already committed, so a mid-stream encoding failure
	// cannot be reported to the client. Log it instead of discarding it.
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writing JSON response", "error", err)
	}
}

// WriteError translates err into a status code and a client-safe body. The
// underlying cause is logged, never serialized, so driver and query detail
// cannot leak to callers.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	kind := apperr.KindOf(err)
	status := StatusFor(kind)
	requestID := server.RequestIDFrom(r.Context())

	if status >= http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "request failed",
			"error", err,
			"method", r.Method,
			"path", r.URL.Path,
			"request_id", requestID,
		)
	}

	WriteJSON(w, status, ErrorResponse{
		Error:     apperr.Message(err),
		Code:      kind.String(),
		RequestID: requestID,
	})
}

// DecodeJSON reads a JSON request body into dst. It rejects oversized bodies
// and unknown fields, and returns an apperr.KindInvalid error on failure so
// callers can pass it straight to WriteError.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return apperr.Wrap(apperr.KindInvalid, "request body is not valid JSON", err)
	}
	return nil
}
