package service

import "errors"

var (
	ErrForbidden    = errors.New("forbidden: insufficient permissions")
	ErrUnauthorized = errors.New("unauthorized")
)
