// Package handler is the transport layer for the football service.
//
// Handlers decode input, delegate to a service, and encode the result. They
// contain no authorization logic: every write rule lives in the service layer
// (see internal/service/football_service.go), so there is exactly one place to
// read and to change. Duplicating the checks here previously let the two
// copies drift apart.
package handler

import (
	"net/http"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/platform/httpx"
)

// pageFrom reads the standard limit/offset parameters and clamps them.
func pageFrom(r *http.Request) (domain.Page, error) {
	limit, err := httpx.QueryInt(r, "limit", domain.DefaultPageSize)
	if err != nil {
		return domain.Page{}, err
	}

	offset, err := httpx.QueryInt(r, "offset", 0)
	if err != nil {
		return domain.Page{}, err
	}

	return domain.NewPage(limit, offset), nil
}
