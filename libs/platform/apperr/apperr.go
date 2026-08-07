// Package apperr defines the error taxonomy shared by every ScoutPulse
// service.
//
// The point of this package is to let the service layer say *what kind* of
// failure happened without knowing anything about HTTP, and to let the
// transport layer turn that into a status code without a hand-written switch
// in every handler. It also enforces the split between the message a client
// may see and the cause only the logs may see.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies a failure. Transport layers map it to a protocol-specific
// status; see platform/server.
type Kind int

const (
	// KindInternal is the zero value: an unexpected failure. Its cause is
	// never shown to clients.
	KindInternal Kind = iota
	KindInvalid
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
)

func (k Kind) String() string {
	switch k {
	case KindInvalid:
		return "invalid"
	case KindUnauthorized:
		return "unauthorized"
	case KindForbidden:
		return "forbidden"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	default:
		return "internal"
	}
}

// Error carries a client-safe message alongside the internal cause.
type Error struct {
	Kind Kind
	// Message is safe to return to a client. It must never embed driver or
	// query detail.
	Message string
	// Cause is the underlying error. It is logged, never serialized.
	Cause error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// New builds an Error with no underlying cause.
func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

// Wrap attaches a kind and a client-safe message to an existing error.
func Wrap(kind Kind, message string, cause error) *Error {
	return &Error{Kind: kind, Message: message, Cause: cause}
}

func Invalid(message string) *Error      { return New(KindInvalid, message) }
func Unauthorized(message string) *Error { return New(KindUnauthorized, message) }
func Forbidden(message string) *Error    { return New(KindForbidden, message) }
func NotFound(message string) *Error     { return New(KindNotFound, message) }
func Conflict(message string) *Error     { return New(KindConflict, message) }

// Internal wraps an unexpected failure. The cause is preserved for logging
// but the client-facing message stays generic.
func Internal(cause error) *Error {
	return &Error{Kind: KindInternal, Message: "internal server error", Cause: cause}
}

// KindOf reports the Kind of err, walking the wrap chain. Errors that did not
// originate in this package are treated as KindInternal.
func KindOf(err error) Kind {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return KindInternal
}

// Message returns the client-safe message for err, falling back to a generic
// string for errors that did not originate in this package. This is what
// prevents driver detail from reaching a response body.
func Message(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return "internal server error"
}
