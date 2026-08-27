// Package handler is the transport layer for the football service.
//
// Handlers decode input, delegate to a service, and encode the result. They
// contain no authorization logic: every write rule lives in the service layer
// (see internal/service/football_service.go), so there is exactly one place to
// read and to change. Duplicating the checks here previously let the two
// copies drift apart.
package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/platform/apperr"
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

// maxIDsPerRequest bounds a batch lookup.
//
// An unbounded ?ids= list is a denial-of-service shape: the query text and the
// argument array both grow with it, and the caller controls the size. The cap
// matches domain.MaxPageSize, since more ids than a page can return is a
// request that cannot be answered in full anyway.
const maxIDsPerRequest = domain.MaxPageSize

// idsFrom reads a comma-separated id list, e.g. ?ids=uuid1,uuid2.
//
// Blank entries are dropped rather than passed through, so a trailing comma or
// a doubled separator is not turned into a lookup for the empty string -- which
// Postgres rejects as a malformed uuid, producing a 400 that blames the caller
// for a stray character.
func idsFrom(r *http.Request, name string) ([]string, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}

	var ids []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}

	if len(ids) > maxIDsPerRequest {
		return nil, apperr.Invalid(fmt.Sprintf("at most %d ids per request", maxIDsPerRequest))
	}
	return ids, nil
}
