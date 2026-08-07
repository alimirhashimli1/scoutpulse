package httpx

import (
	"net/http"
	"strconv"

	"github.com/scoutpulse/libs/platform/apperr"
)

// QueryInt reads an integer query parameter. A missing or empty parameter
// yields def; a malformed one is an apperr.KindInvalid error rather than a
// silent fallback, so typos surface instead of being ignored.
func QueryInt(r *http.Request, name string, def int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apperr.Wrap(apperr.KindInvalid, "invalid value for "+name, err)
	}
	return v, nil
}

// QueryBool reads an optional boolean query parameter. It returns nil when the
// parameter is absent, which callers use to distinguish "unset" from "false".
func QueryBool(r *http.Request, name string) (*bool, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}

	v, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, apperr.Wrap(apperr.KindInvalid, "invalid value for "+name, err)
	}
	return &v, nil
}

// QueryString reads an optional string query parameter, returning nil when it
// is absent or empty.
func QueryString(r *http.Request, name string) *string {
	if raw := r.URL.Query().Get(name); raw != "" {
		return &raw
	}
	return nil
}
