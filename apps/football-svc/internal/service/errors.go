package service

import "github.com/scoutpulse/libs/platform/apperr"

// Authorization outcomes. These carry an apperr.Kind, so the transport layer
// maps them to a status code without a per-handler switch.
var (
	ErrForbidden    = apperr.Forbidden("insufficient permissions")
	ErrUnauthorized = apperr.Unauthorized("authentication required")
	ErrNotFound     = apperr.NotFound("not found")
)
